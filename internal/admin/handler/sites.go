package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// siteDTO 站点返回结构，把 Site 模型中的 JSON 列解析成数组后输出。
type siteDTO struct {
	ID           uint     `json:"id"`             // 站点 ID
	Name         string   `json:"name"`           // 站点名称
	Hosts        []string `json:"hosts"`          // 匹配的域名列表
	Upstream     string   `json:"upstream"`       // 回源上游地址（http(s)://host:port）
	Enabled      int      `json:"enabled"`        // 是否启用：1 启用，0 停用
	DetectBody   int      `json:"detectBody"`     // 是否检测请求体：1 检测，0 跳过
	MaxBodyBytes int      `json:"maxBodyBytes"`   // 请求体检测上限（字节）
	Whitelist    []string `json:"whitelistPaths"` // 不做检测的路径前缀白名单
	// ProtectCfg 站点级防护覆盖：null=跟随全局。enabled=1 时按字段覆盖全局。
	ProtectCfg *model.SiteProtect `json:"protectCfg"`
	// BehindCDN 空字符串表示直连；'cloudflare' 表示站点在 CF 橙云后面。
	BehindCDN string `json:"behindCdn"`
	CreatedAt int64  `json:"createdAt"` // 创建时间（毫秒时间戳）
	UpdatedAt int64  `json:"updatedAt"` // 更新时间（毫秒时间戳）
}

// toSiteDTO 把站点模型转换为返回结构：解析 hosts/whitelistPath 两个 JSON 列，
// protect_cfg 列解析失败时置为 null（前端按跟随全局展示）。
// 参数 s：站点模型记录。
// 返回：siteDTO；数组字段保证非 nil（无数据时为空切片）。
func toSiteDTO(s model.Site) siteDTO {
	var hosts, wl []string
	_ = json.Unmarshal([]byte(s.Hosts), &hosts)
	_ = json.Unmarshal([]byte(s.WhitelistPath), &wl)
	if hosts == nil {
		hosts = []string{}
	}
	if wl == nil {
		wl = []string{}
	}
	var protect *model.SiteProtect
	if p, err := model.ParseSiteProtect(s.ProtectCfg); err == nil {
		protect = p
	}
	return siteDTO{
		ID: s.ID, Name: s.Name, Hosts: hosts, Upstream: s.Upstream,
		Enabled: s.Enabled, DetectBody: s.DetectBody, MaxBodyBytes: s.MaxBodyBytes,
		Whitelist: wl, ProtectCfg: protect, BehindCDN: s.BehindCDN,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

// SiteList 查询站点列表（不分页返回全部，站点数量少）。
// 路由：GET /api/sites（需登录）。
// 参数 c：Gin 上下文（无查询参数）。
// 返回：无（200 返回 {"records":[...],"total":n}；数据库失败返回 500）。
func (s *Server) SiteList(c *gin.Context) {
	var rows []model.Site
	if err := s.Store.Gorm().Order("id ASC").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	list := make([]siteDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, toSiteDTO(r))
	}
	response.OK(c, gin.H{"records": list, "total": len(list)})
}

// siteInput 新增/修改站点的入参，两个接口共用。
type siteInput struct {
	Name         string   `json:"name" binding:"required"`     // 站点名称：required 表示必填
	Hosts        []string `json:"hosts"`                       // 匹配域名列表：落库前统一小写去空白
	Upstream     string   `json:"upstream" binding:"required"` // 回源上游地址：required 表示必填，且须通过 validUpstream 校验
	Enabled      *int     `json:"enabled"`                     // 是否启用：指针类型，不传时新增默认 1、修改保持原值
	DetectBody   *int     `json:"detectBody"`                  // 是否检测请求体：指针类型，不传时新增默认 1、修改保持原值
	MaxBodyBytes int      `json:"maxBodyBytes"`                // 请求体检测上限（字节）：<=0 时新增回落默认值、修改保持原值
	Whitelist    []string `json:"whitelistPaths"`              // 不做检测的路径前缀白名单
	// ProtectCfg 指针类型：不传时保持原值；传 null 或 {enabled:0} 表示改回跟随全局。
	ProtectCfg *model.SiteProtect `json:"protectCfg"`
	// 指针类型：不传时保持原值，兼容旧版前端；传空串表示改回直连。
	BehindCDN *string `json:"behindCdn"`
}

// protectCfgStr 把站点防护覆盖配置序列化为落库 JSON 字符串。
// 参数 p：防护覆盖配置指针。
// 返回：落库字符串（空串表示跟随全局）与可能的序列化/校验错误。
//
// p 为 nil 或 enabled 不为 1 时返回空串（跟随全局）；非空结果会再经 ParseSiteProtect 校验一遍，非法配置直接拒绝。
func protectCfgStr(p *model.SiteProtect) (string, error) {
	if p == nil || p.Enabled != 1 {
		return "", nil // 跟随全局
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	// 走一遍解析校验，非法配置直接拒绝
	if _, err := model.ParseSiteProtect(string(b)); err != nil {
		return "", err
	}
	return string(b), nil
}

// normalizeBehindCDN 归一化 CDN 标记：只认 'cloudflare'，其余一律收敛为空串（直连）。
// 参数 v：原始标记值。
// 返回：'cloudflare' 或 ”。规则见 model.NormalizeBehindCDN，这里只是缩短调用点。
func normalizeBehindCDN(v string) string { return model.NormalizeBehindCDN(v) }

// SiteCreate 新增站点，保存成功后触发配置热加载，使新站点立即生效。
// 路由：POST /api/sites（写操作组：CSRF 校验、演示只读拦截、审计、site:write 权限）。
// 参数 c：Gin 上下文，JSON 请求体为 siteInput。
// 返回：无（200 返回新站点的 siteDTO；参数、上游地址或防护配置非法返回 400，数据库或热加载失败返回 500）。
func (s *Server) SiteCreate(c *gin.Context) {
	var in siteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "名称与上游地址必填")
		return
	}
	if !validUpstream(in.Upstream) {
		response.BadRequest(c, "上游必须是 http(s)://host:port，且不能指向控制面 8899")
		return
	}

	now := store.NowMS()
	hosts, _ := json.Marshal(normalizeHosts(in.Hosts))
	wl, _ := json.Marshal(in.Whitelist)
	protectStr := ""
	if in.ProtectCfg != nil {
		var err error
		if protectStr, err = protectCfgStr(in.ProtectCfg); err != nil {
			response.BadRequest(c, "防护配置非法: "+err.Error())
			return
		}
	}
	row := model.Site{
		Name: in.Name, Hosts: string(hosts), Upstream: in.Upstream,
		Enabled: 1, DetectBody: 1, MaxBodyBytes: in.MaxBodyBytes,
		WhitelistPath: string(wl), RateLimit: "{}", ProtectCfg: protectStr,
		CreatedAt: now, UpdatedAt: now,
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if in.DetectBody != nil {
		row.DetectBody = *in.DetectBody
	}
	if in.BehindCDN != nil {
		row.BehindCDN = normalizeBehindCDN(*in.BehindCDN)
	}
	// 请求体上限未指定或非法时，回落为默认 256KB
	if row.MaxBodyBytes <= 0 {
		row.MaxBodyBytes = 262144
	}
	if err := s.Store.Gorm().Create(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已保存但重载失败: "+err.Error())
		return
	}
	// 把操作对象写入请求上下文，供审计中间件落库
	c.Set(middleware.CtxAuditObj, "site")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(uint64(row.ID), 10))
	response.OK(c, toSiteDTO(row))
}

// SiteUpdate 修改站点，站点改名时级联更新引用旧站点名的黑白名单与封禁记录。
// 路由：PUT /api/sites/:id（写操作组：CSRF 校验、演示只读拦截、审计、site:write 权限）。
// 参数 c：Gin 上下文，路径参数 id 为站点 ID，JSON 请求体为 siteInput。
// 返回：无（200 返回更新后的 siteDTO；id 非法、站点不存在或参数非法返回 400/404，数据库或热加载失败返回 500）。
func (s *Server) SiteUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var row model.Site
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		response.NotFound(c, "站点不存在")
		return
	}
	var in siteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if !validUpstream(in.Upstream) {
		response.BadRequest(c, "上游地址非法")
		return
	}

	hosts, _ := json.Marshal(normalizeHosts(in.Hosts))
	wl, _ := json.Marshal(in.Whitelist)
	oldName := row.Name
	row.Name = in.Name
	row.Hosts = string(hosts)
	row.Upstream = in.Upstream
	row.WhitelistPath = string(wl)
	// 0 视为未传，保持原值
	if in.MaxBodyBytes > 0 {
		row.MaxBodyBytes = in.MaxBodyBytes
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if in.DetectBody != nil {
		row.DetectBody = *in.DetectBody
	}
	if in.BehindCDN != nil {
		row.BehindCDN = normalizeBehindCDN(*in.BehindCDN)
	}
	if in.ProtectCfg != nil {
		ps, err := protectCfgStr(in.ProtectCfg)
		if err != nil {
			response.BadRequest(c, "防护配置非法: "+err.Error())
			return
		}
		row.ProtectCfg = ps
	}
	row.UpdatedAt = store.NowMS()

	if err := s.Store.Gorm().Save(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	// 黑白名单与按站点封禁存的是站点名，改名必须级联，否则这些规则会静默失效
	if oldName != row.Name && oldName != "" {
		if err := s.Store.Gorm().Model(&model.IPRule{}).Where("site = ?", oldName).
			Update("site", row.Name).Error; err != nil {
			s.failDB(c, err)
			return
		}
		if err := s.Store.Gorm().Model(&model.Ban{}).Where("site = ?", oldName).
			Update("site", row.Name).Error; err != nil {
			s.failDB(c, err)
			return
		}
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已保存但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "site")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, toSiteDTO(row))
}

// SiteDelete 删除站点，并清理挂在站点名下的 IP 名单与未解除的封禁记录。
// 路由：DELETE /api/sites/:id（写操作组：CSRF 校验、演示只读拦截、审计、site:write 权限）。
// 参数 c：Gin 上下文，路径参数 id 为站点 ID。
// 返回：无（200 返回 {"ok":true}；id 非法或站点不存在返回 400/404，数据库或热加载失败返回 500）。
func (s *Server) SiteDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var row model.Site
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		response.NotFound(c, "站点不存在")
		return
	}
	if err := s.Store.Gorm().Delete(&model.Site{}, id).Error; err != nil {
		s.failDB(c, err)
		return
	}
	// 站点没了，挂在它身上的名单和封禁就是死数据，一并清掉免得列表里出现幽灵站点
	if err := s.Store.Gorm().Where("site = ?", row.Name).Delete(&model.IPRule{}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.Store.Gorm().Model(&model.Ban{}).
		Where("site = ? AND released_at = 0", row.Name).
		Update("released_at", store.NowMS()).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已删除但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "site")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, gin.H{"ok": true})
}

// SiteTest 对站点上游做连通性测试：请求上游根路径，返回状态码与耗时。
// 路由：POST /api/sites/:id/test（写操作组：CSRF 校验、演示只读拦截、审计、site:write 权限）。
// 参数 c：Gin 上下文，路径参数 id 为站点 ID。
// 返回：无（200 返回 ok/status/latencyMs/server；请求失败时 ok=false 并附 error 文案，id 非法或站点不存在返回 400/404）。
func (s *Server) SiteTest(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var row model.Site
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		response.NotFound(c, "站点不存在")
		return
	}

	// 5 秒超时，避免测试请求被无响应的上游拖死
	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Get(strings.TrimSuffix(row.Upstream, "/") + "/")
	if err != nil {
		response.OK(c, gin.H{"ok": false, "error": err.Error(),
			"latencyMs": time.Since(start).Milliseconds()})
		return
	}
	defer resp.Body.Close()

	// 5xx 视为上游异常，4xx 及以下视为连通（403/404 等至少说明 TCP/HTTP 链路可达）
	response.OK(c, gin.H{
		"ok":        resp.StatusCode < 500,
		"status":    resp.StatusCode,
		"latencyMs": time.Since(start).Milliseconds(),
		"server":    resp.Header.Get("Server"),
	})
}

// validUpstream 校验回源上游地址：必须以 http:// 或 https:// 开头，且不能包含 ":8899"（禁止指向控制面自身端口）。
// 参数 u：上游地址。
// 返回：true 表示合法。校验规则与 config 包保持同一套。
func validUpstream(u string) bool {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return false
	}
	return !strings.Contains(u, ":8899")
}

// normalizeHosts 归一化域名列表：转小写、去首尾空白并丢弃空项。
// 参数 in：原始域名列表。
// 返回：归一化后的非空域名列表（保证非 nil）。
func normalizeHosts(in []string) []string {
	out := make([]string, 0, len(in))
	for _, h := range in {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}
