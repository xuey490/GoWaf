package handler

import (
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

// ipRuleDTO IP 名单返回结构。
type ipRuleDTO struct {
	ID        uint   `json:"id"`
	IP        string `json:"ip"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	Reason    string `json:"reason"`
	ExpiresAt int64  `json:"expiresAt"`
	CreatedBy string `json:"createdBy"`
	CreatedAt int64  `json:"createdAt"`
}

// IPList 名单列表。
func (s *Server) IPList(c *gin.Context) {
	current, size := pageParams(c)
	kind := c.Query("kind")
	kw := strings.TrimSpace(c.Query("keyword"))

	q := s.Store.Gorm().Model(&model.IPRule{})
	if kind != "" {
		q = q.Where("kind = ?", kind)
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
			ID: r.ID, IP: r.IP, Kind: r.Kind, Scope: r.Scope, Reason: r.Reason,
			ExpiresAt: r.ExpiresAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		})
	}
	response.OK(c, response.NewPage(list, current, size, total))
}

// ipRuleInput 新增/修改入参。
type ipRuleInput struct {
	IP        string   `json:"ip"`
	IPs       []string `json:"ips"`
	Kind      string   `json:"kind"`
	Scope     string   `json:"scope"`
	Reason    string   `json:"reason"`
	ExpiresAt int64    `json:"expiresAt"`
}

// IPCreate 支持单条或批量。
func (s *Server) IPCreate(c *gin.Context) {
	var in ipRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
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
	if in.Scope == "" {
		in.Scope = "global"
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
			IP: raw, Kind: in.Kind, Scope: in.Scope, Reason: in.Reason,
			ExpiresAt: in.ExpiresAt, CreatedBy: operator, CreatedAt: now,
		}
		// 唯一索引冲突时视为已存在，跳过
		if err := s.Store.Gorm().Where(model.IPRule{IP: raw, Kind: in.Kind, Scope: in.Scope}).
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

// IPUpdate 修改。
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

// IPDelete 删除。
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

// IPImport 批量导入，每行一个 IP/CIDR，可带 # 注释。
func (s *Server) IPImport(c *gin.Context) {
	var in struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		response.BadRequest(c, "text 不能为空")
		return
	}
	kind := in.Kind
	if kind != "black" && kind != "white" {
		kind = "black"
	}

	operator := c.GetString(middleware.CtxUserName)
	now := store.NowMS()
	created, skipped := 0, 0
	for _, line := range strings.Split(in.Text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.IndexAny(line, " \t"); i > 0 {
			line = line[:i]
		}
		if !validIPCandidate(line) {
			skipped++
			continue
		}
		row := model.IPRule{IP: line, Kind: kind, Scope: "global", Reason: "批量导入",
			CreatedBy: operator, CreatedAt: now}
		res := s.Store.Gorm().Where(model.IPRule{IP: line, Kind: kind, Scope: "global"}).
			FirstOrCreate(&row)
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

// validIPCandidate 校验单 IP 或 CIDR。
func validIPCandidate(s string) bool {
	if strings.Contains(s, "/") {
		_, err := netip.ParsePrefix(s)
		return err == nil
	}
	_, err := netip.ParseAddr(s)
	return err == nil
}

// ===== 封禁 =====

// BanList 封禁列表。内存表为准，DB 只用于展示历史。
func (s *Server) BanList(c *gin.Context) {
	current, size := pageParams(c)
	active := c.DefaultQuery("active", "1") == "1"

	q := s.Store.Gorm().Model(&model.Ban{})
	if active {
		now := store.NowMS()
		q = q.Where("released_at = 0 AND (expires_at = 0 OR expires_at > ?)", now)
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
		if s.Cfg.Demo.Enabled && s.Cfg.Demo.MaskIPs {
			ip = maskIP(ip)
		}
		list = append(list, gin.H{
			"id": r.ID, "ip": ip, "reason": r.Reason, "source": r.Source,
			"hitCount": r.HitCount, "createdAt": r.CreatedAt,
			"expiresAt": r.ExpiresAt, "releasedAt": r.ReleasedAt,
		})
	}
	response.OK(c, response.NewPage(list, current, size, total))
}

// BanCreate 手动封禁。
func (s *Server) BanCreate(c *gin.Context) {
	var in struct {
		IP       string `json:"ip"`
		Duration int64  `json:"duration"` // 秒，0 表示永久
		Reason   string `json:"reason"`
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

	now := store.NowMS()
	expires := int64(0)
	if in.Duration > 0 {
		expires = now + in.Duration*1000
	}
	operator := c.GetString(middleware.CtxUserName)

	row := model.Ban{
		IP: in.IP, Reason: in.Reason, Source: "manual",
		HitCount: 1, CreatedAt: now, ExpiresAt: expires,
	}
	if err := s.Store.Gorm().Create(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.RT.Bans.Ban(in.IP, expires)
	metrics.BannedGauge.Set(float64(s.RT.Bans.Size()))

	c.Set(middleware.CtxAuditObj, "ban")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(uint64(row.ID), 10))
	response.OK(c, gin.H{"id": row.ID, "expiresAt": expires, "operator": operator})
}

// BanDelete 解封。
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
	s.RT.Bans.Unban(row.IP)
	// 手动解封 = 给一次干净的机会：把这个 IP 名下所有未释放的自动封禁记录一起释放，
	// 连击数按「released_at = 0 的 auto 记录」统计，于是自然归零。
	// （只释放这一条的话，前两次的旧记录还留着，下次命中立刻又判永久，救不回来。）
	if err := s.Store.Gorm().Model(&model.Ban{}).
		Where("ip = ? AND source = ? AND released_at = 0", row.IP, "auto").
		Update("released_at", store.NowMS()).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.RT.Hits.Reset(row.IP)
	metrics.BannedGauge.Set(float64(s.RT.Bans.Size()))

	c.Set(middleware.CtxAuditObj, "ban")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, gin.H{"ok": true})
}

// BanClear 清空所有临时封禁。
func (s *Server) BanClear(c *gin.Context) {
	now := store.NowMS()
	if err := s.Store.Gorm().Model(&model.Ban{}).
		Where("released_at = 0").
		Update("released_at", now).Error; err != nil {
		s.failDB(c, err)
		return
	}
	n := s.RT.Bans.Clear()
	metrics.BannedGauge.Set(0)

	c.Set(middleware.CtxAuditObj, "ban")
	response.OK(c, gin.H{"released": n})
}
