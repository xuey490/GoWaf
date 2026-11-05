package handler

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/metrics"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// ===== IP 名单（黑/白名单） =====

// 作用域取值：IP 名单（ip_rules）与封禁（bans）共用。
const (
	scopeGlobal = "global" // 全局作用域：对所有站点生效
	scopeSite   = "site"   // 站点作用域：仅对指定站点生效
)

// normalizeScope 校验并归一化作用域。
//
// 规则：scope=site 必须带一个真实存在的站点名；scope=global 忽略站点。
// 只传 site 不传 scope 时按 site 处理，兼容老前端/脚本调用。
// 参数 scope：前端传入的作用域（global/site，允许为空，为空时按 site 推断）。
// 参数 site：站点名（允许为空）。
// 返回：归一化后的 (scope, site)；作用域非法或站点不存在时返回错误。
func (s *Server) normalizeScope(scope, site string) (string, string, error) {
	scope = strings.TrimSpace(scope)
	site = strings.TrimSpace(site)
	if scope == "" {
		if site != "" {
			scope = scopeSite
		} else {
			scope = scopeGlobal
		}
	}
	switch scope {
	case scopeGlobal:
		return scopeGlobal, "", nil
	case scopeSite:
		if site == "" {
			return "", "", fmt.Errorf("按站点作用域必须选择站点")
		}
		var n int64
		if err := s.Store.Gorm().Model(&model.Site{}).Where("name = ?", site).
			Count(&n).Error; err != nil {
			return "", "", err
		}
		if n == 0 {
			return "", "", fmt.Errorf("站点不存在: %s", site)
		}
		return scopeSite, site, nil
	}
	return "", "", fmt.Errorf("scope 只能是 global 或 site")
}

// ipRuleDTO IP 名单返回结构，字段与前端名单表格列一一对应。
type ipRuleDTO struct {
	ID        uint   `json:"id"`        // 名单记录 ID
	IP        string `json:"ip"`        // 单个 IP 或 CIDR 网段
	Kind      string `json:"kind"`      // 名单类型：black 黑名单 / white 白名单
	Scope     string `json:"scope"`     // 作用域：global 全局 / site 站点
	Site      string `json:"site"`      // 作用域为 site 时的站点名，全局时为空
	Reason    string `json:"reason"`    // 添加原因备注
	ExpiresAt int64  `json:"expiresAt"` // 过期时间（毫秒时间戳），0 表示永不过期
	CreatedBy string `json:"createdBy"` // 创建人（管理员用户名）
	CreatedAt int64  `json:"createdAt"` // 创建时间（毫秒时间戳）
}

// IPList 分页查询 IP 名单。
// 参数 c：Gin 上下文，支持 current/size 分页参数，以及 kind（black/white）、site（站点名）、keyword（IP 模糊匹配）过滤参数。
// 返回：无（200 返回分页 JSON：list、current、size、total；数据库失败返回 500）。
// 路由：GET /api/ips（仅需登录）。
func (s *Server) IPList(c *gin.Context) {
	current, size := pageParams(c)
	kind := c.Query("kind")
	kw := strings.TrimSpace(c.Query("keyword"))
	site := strings.TrimSpace(c.Query("site"))

	q := s.Store.Gorm().Model(&model.IPRule{})
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if site != "" {
		q = q.Where("site = ?", site)
	}
	if kw != "" {
		q = q.Where("ip LIKE ?", "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		s.failDB(c, err)
		return
	}
	var rows []model.IPRule
	if err := q.Order("id DESC").Offset((current - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	list := make([]ipRuleDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, ipRuleDTO{
			ID: r.ID, IP: r.IP, Kind: r.Kind, Scope: r.Scope, Site: r.Site, Reason: r.Reason,
			ExpiresAt: r.ExpiresAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		})
	}
	response.OK(c, response.NewPage(list, current, size, total))
}

// ipRuleInput IP 名单新增/修改入参。
type ipRuleInput struct {
	IP        string   `json:"ip"`        // 单条 IP/CIDR；与 ips 可任选或同时提供
	IPs       []string `json:"ips"`       // 批量添加的 IP/CIDR 列表
	Kind      string   `json:"kind"`      // 名单类型：black 黑名单 / white 白名单；新增时必填，修改时留空表示不改
	Scope     string   `json:"scope"`     // 作用域：global / site，留空时按 site 是否非空推断
	Site      string   `json:"site"`      // 作用域为 site 时的站点名
	Reason    string   `json:"reason"`    // 添加原因备注
	ExpiresAt int64    `json:"expiresAt"` // 过期时间（毫秒时间戳），0 表示永久
}

// IPCreate 新增 IP 名单，支持单条或批量。
// 参数 c：Gin 上下文，JSON 体为 ipRuleInput，ip（单条）与 ips（批量）至少提供一个。
// 返回：无（200 返回 {"created": 成功处理的条数，已存在的重复项复用原记录并同样计入}；参数非法返回 400，数据库失败返回 500，保存成功但热重载失败返回 500 并提示）。
// 路由：POST /api/ips（需 ip:ban 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) IPCreate(c *gin.Context) {
	var in ipRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	// 单条与批量合并成同一个列表处理
	items := in.IPs
	if in.IP != "" {
		items = append(items, in.IP)
	}
	if len(items) == 0 {
		response.BadRequest(c, "至少提供一个 IP")
		return
	}
	if in.Kind != "black" && in.Kind != "white" {
		response.BadRequest(c, "kind 只能是 black 或 white")
		return
	}
	scope, site, err := s.normalizeScope(in.Scope, in.Site)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	operator := c.GetString(middleware.CtxUserName)
	now := store.NowMS()
	created := 0
	for _, raw := range items {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !validIPCandidate(raw) {
			response.BadRequest(c, "非法 IP 或 CIDR: "+raw)
			return
		}
		row := model.IPRule{
			IP: raw, Kind: in.Kind, Scope: scope, Site: site, Reason: in.Reason,
			ExpiresAt: in.ExpiresAt, CreatedBy: operator, CreatedAt: now,
		}
		// 按 IP+类型+作用域+站点查重：已存在则复用原记录，不存在才创建
		if err := s.Store.Gorm().Where(model.IPRule{IP: raw, Kind: in.Kind, Scope: scope, Site: site}).
			FirstOrCreate(&row).Error; err != nil {
			s.failDB(c, err)
			return
		}
		created++
	}

	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "名单已保存但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "ip_rule")
	response.OK(c, gin.H{"created": created})
}

// IPUpdate 修改单条 IP 名单记录。
// 参数 c：Gin 上下文，路径参数 id 为记录 ID；JSON 体为 ipRuleInput，仅非空字段参与更新。
// 返回：无（200 返回 {"ok":true}；id 非法、参数非法或没有可更新字段返回 400，数据库失败返回 500，保存成功但热重载失败返回 500 并提示）。
// 路由：PUT /api/ips/:id（需 ip:ban 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) IPUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var in ipRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	updates := map[string]any{}
	if in.IP != "" {
		if !validIPCandidate(in.IP) {
			response.BadRequest(c, "非法 IP 或 CIDR")
			return
		}
		updates["ip"] = in.IP
	}
	if in.Kind != "" {
		if in.Kind != "black" && in.Kind != "white" {
			response.BadRequest(c, "kind 非法")
			return
		}
		updates["kind"] = in.Kind
	}
	if in.Reason != "" {
		updates["reason"] = in.Reason
	}
	if in.Scope != "" || in.Site != "" {
		// 编辑弹窗总会带上作用域；改成全局时顺便把 site 清空，避免留下孤儿关联
		scope, site, err := s.normalizeScope(in.Scope, in.Site)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		updates["scope"] = scope
		updates["site"] = site
	}
	// 过期时间非负即更新：0 表示改为永久，传负数可跳过该项修改
	if in.ExpiresAt >= 0 {
		updates["expires_at"] = in.ExpiresAt
	}
	if len(updates) == 0 {
		response.BadRequest(c, "没有需要修改的字段")
		return
	}

	if err := s.Store.Gorm().Model(&model.IPRule{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已保存但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "ip_rule")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, gin.H{"ok": true})
}

// IPDelete 删除单条 IP 名单记录。
// 参数 c：Gin 上下文，路径参数 id 为记录 ID（记录不存在时同样返回成功）。
// 返回：无（200 返回 {"ok":true}；id 非法返回 400，数据库失败返回 500，删除成功但热重载失败返回 500 并提示）。
// 路由：DELETE /api/ips/:id（需 ip:ban 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) IPDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	if err := s.Store.Gorm().Delete(&model.IPRule{}, id).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已删除但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "ip_rule")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, gin.H{"ok": true})
}

// IPImport 批量导入 IP 名单。
// 参数 c：Gin 上下文，JSON 体含 text（多行文本，每行一个 IP/CIDR，支持 # 注释行）、kind、scope、site。
// 返回：无（200 返回 {"created": 新增条数, "skipped": 跳过条数（非法行或已存在）}；text 为空或作用域非法返回 400，保存成功但热重载失败返回 500 并提示）。
// 路由：POST /api/ips/import（需 ip:ban 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) IPImport(c *gin.Context) {
	var in struct {
		Text  string `json:"text"`  // 多行导入文本，每行一个 IP/CIDR
		Kind  string `json:"kind"`  // 名单类型：black/white，非法值按 black 处理
		Scope string `json:"scope"` // 作用域：global/site
		Site  string `json:"site"`  // 作用域为 site 时的站点名
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		response.BadRequest(c, "text 不能为空")
		return
	}
	kind := in.Kind
	if kind != "black" && kind != "white" {
		kind = "black"
	}
	scope, site, err := s.normalizeScope(in.Scope, in.Site)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	operator := c.GetString(middleware.CtxUserName)
	now := store.NowMS()
	created, skipped := 0, 0
	for _, line := range strings.Split(in.Text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 截掉行内空白及之后的备注，如 "1.2.3.4 # 说明" 只保留 "1.2.3.4"
		if i := strings.IndexAny(line, " \t"); i > 0 {
			line = line[:i]
		}
		if !validIPCandidate(line) {
			skipped++
			continue
		}
		row := model.IPRule{IP: line, Kind: kind, Scope: scope, Site: site,
			Reason: "批量导入", CreatedBy: operator, CreatedAt: now}
		res := s.Store.Gorm().Where(model.IPRule{IP: line, Kind: kind, Scope: scope, Site: site}).
			FirstOrCreate(&row)
		// 查询出错或未新增行（已存在）都计入 skipped
		if res.Error != nil || res.RowsAffected == 0 {
			skipped++
			continue
		}
		created++
	}

	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已导入但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "ip_rule")
	response.OK(c, gin.H{"created": created, "skipped": skipped})
}

// validIPCandidate 校验文本是否为合法的单个 IP 或 CIDR 网段。
// 参数 s：待校验的 IP/CIDR 文本。
// 返回：合法返回 true，否则返回 false；含 "/" 时按 CIDR 前缀解析，否则按单个 IP 解析。
func validIPCandidate(s string) bool {
	if strings.Contains(s, "/") {
		_, err := netip.ParsePrefix(s)
		return err == nil
	}
	_, err := netip.ParseAddr(s)
	return err == nil
}

// ===== 封禁 =====

// BanList 分页查询封禁记录。
// 参数 c：Gin 上下文，支持 current/size 分页；active=1（默认）只查未释放且未过期的生效记录，site 按站点过滤。
// 返回：无（200 返回分页 JSON；演示模式开启 MaskIPs 时客户端 IP 打码；数据库失败返回 500）。
// 路由：GET /api/bans（仅需登录）。
//
// 说明：运行时实际拦截以内存封禁表为准，DB 记录用于展示与追溯历史。
func (s *Server) BanList(c *gin.Context) {
	current, size := pageParams(c)
	active := c.DefaultQuery("active", "1") == "1" // 默认只查生效中（未释放且未过期）的记录
	site := strings.TrimSpace(c.Query("site"))

	q := s.Store.Gorm().Model(&model.Ban{})
	if active {
		now := store.NowMS()
		q = q.Where("released_at = 0 AND (expires_at = 0 OR expires_at > ?)", now)
	}
	if site != "" {
		q = q.Where("site = ?", site)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		s.failDB(c, err)
		return
	}
	var rows []model.Ban
	if err := q.Order("id DESC").Offset((current - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		ip := r.IP
		// 演示模式下按配置对客户端 IP 打码
		if s.Cfg.Demo.Enabled && s.Cfg.Demo.MaskIPs {
			ip = maskIP(ip)
		}
		list = append(list, gin.H{
			"id": r.ID, "ip": ip, "reason": r.Reason, "source": r.Source,
			"scope": r.Scope, "site": r.Site,
			"hitCount": r.HitCount, "createdAt": r.CreatedAt,
			"expiresAt": r.ExpiresAt, "releasedAt": r.ReleasedAt,
		})
	}
	response.OK(c, response.NewPage(list, current, size, total))
}

// BanCreate 手动封禁单个 IP。scope=site 时只对指定站点生效，默认全局。
// 参数 c：Gin 上下文，JSON 体含 ip（必填，仅支持单个 IP）、duration（秒，0 表示永久）、reason、scope、site。
// 返回：无（200 返回 {"id","expiresAt","scope","site","operator"}；参数非法返回 400，数据库失败返回 500）。
// 路由：POST /api/bans（需 ip:ban 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) BanCreate(c *gin.Context) {
	var in struct {
		IP       string `json:"ip"`       // 要封禁的单个 IP，不支持 CIDR
		Duration int64  `json:"duration"` // 封禁时长（秒），0 表示永久
		Reason   string `json:"reason"`   // 封禁原因备注
		Scope    string `json:"scope"`    // 作用域：global/site
		Site     string `json:"site"`     // 作用域为 site 时的站点名
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.IP == "" {
		response.BadRequest(c, "ip 不能为空")
		return
	}
	if !validIPCandidate(in.IP) {
		response.BadRequest(c, "非法 IP")
		return
	}
	// 只接受单 IP，CIDR 请走名单
	if strings.Contains(in.IP, "/") {
		response.BadRequest(c, "封禁仅支持单个 IP，网段请加到黑名单")
		return
	}
	scope, site, err := s.normalizeScope(in.Scope, in.Site)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	now := store.NowMS()
	expires := int64(0) // 0 表示永久
	if in.Duration > 0 {
		expires = now + in.Duration*1000
	}
	operator := c.GetString(middleware.CtxUserName)

	row := model.Ban{
		IP: in.IP, Reason: in.Reason, Source: "manual", Scope: scope, Site: site,
		HitCount: 1, CreatedAt: now, ExpiresAt: expires,
	}
	if err := s.Store.Gorm().Create(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.RT.Bans.Ban(in.IP, site, expires) // 同步写入内存封禁表，立即生效，无需等待重载
	metrics.BannedGauge.Set(float64(s.RT.Bans.Size()))

	c.Set(middleware.CtxAuditObj, "ban")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(uint64(row.ID), 10))
	response.OK(c, gin.H{"id": row.ID, "expiresAt": expires, "scope": scope, "site": site, "operator": operator})
}

// BanDelete 解除一条封禁记录。只解这个作用域，其他站点的同类封禁不受影响。
// 参数 c：Gin 上下文，路径参数 id 为封禁记录 ID。
// 返回：无（200 返回 {"ok":true}；id 非法返回 400，记录不存在返回 404，数据库失败返回 500）。
// 路由：DELETE /api/bans/:id（需 ip:ban 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) BanDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var row model.Ban
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		response.NotFound(c, "封禁记录不存在")
		return
	}
	if err := s.Store.Gorm().Model(&model.Ban{}).Where("id = ?", id).
		Update("released_at", store.NowMS()).Error; err != nil {
		s.failDB(c, err)
		return
	}
	site := ""
	if row.Scope == scopeSite {
		site = row.Site
	}
	s.RT.Bans.Unban(row.IP, site) // 同步释放内存封禁表中的对应条目
	// 手动解封 = 给一次干净的机会：把这个 IP 名下所有未释放的自动封禁记录一起释放，
	// 连击数按「released_at = 0 的 auto 记录」统计，于是自然归零。
	// （只释放这一条的话，前两次的旧记录还留着，下次命中立刻又判永久，救不回来。）
	// 站点封禁不碰自动封禁的账：自动封禁是全局的，解掉一个站点的封禁不该洗白连击数。
	if row.Scope != scopeSite {
		if err := s.Store.Gorm().Model(&model.Ban{}).
			Where("ip = ? AND source = ? AND released_at = 0", row.IP, "auto").
			Update("released_at", store.NowMS()).Error; err != nil {
			s.failDB(c, err)
			return
		}
		s.RT.Hits.Reset(row.IP)
	}
	metrics.BannedGauge.Set(float64(s.RT.Bans.Size()))

	c.Set(middleware.CtxAuditObj, "ban")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, gin.H{"ok": true})
}

// BanClear 一键释放所有未释放的封禁记录。
// 参数 c：Gin 上下文，无请求参数。
// 返回：无（200 返回 {"released": 释放条数}；数据库失败返回 500）。
// 路由：DELETE /api/bans（需 ip:ban 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) BanClear(c *gin.Context) {
	now := store.NowMS()
	if err := s.Store.Gorm().Model(&model.Ban{}).
		Where("released_at = 0").
		Update("released_at", now).Error; err != nil {
		s.failDB(c, err)
		return
	}
	n := s.RT.Bans.Clear() // 同步清空内存封禁表
	metrics.BannedGauge.Set(0)

	c.Set(middleware.CtxAuditObj, "ban")
	response.OK(c, gin.H{"released": n})
}
