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

// siteDTO 站点返回结构。
type siteDTO struct {
	ID           uint     `json:"id"`
	Name         string   `json:"name"`
	Hosts        []string `json:"hosts"`
	Upstream     string   `json:"upstream"`
	Enabled      int      `json:"enabled"`
	DetectBody   int      `json:"detectBody"`
	MaxBodyBytes int      `json:"maxBodyBytes"`
	Whitelist    []string `json:"whitelistPaths"`
	// ProtectCfg 站点级防护覆盖：null=跟随全局。enabled=1 时按字段覆盖全局。
	ProtectCfg *model.SiteProtect `json:"protectCfg"`
	// BehindCDN 空字符串表示直连；'cloudflare' 表示站点在 CF 橙云后面。
	BehindCDN string `json:"behindCdn"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

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

// SiteList 站点列表（不分页，站点数量少）。
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

// siteInput 新增/修改入参。
type siteInput struct {
	Name         string   `json:"name" binding:"required"`
	Hosts        []string `json:"hosts"`
	Upstream     string   `json:"upstream" binding:"required"`
	Enabled      *int     `json:"enabled"`
	DetectBody   *int     `json:"detectBody"`
	MaxBodyBytes int      `json:"maxBodyBytes"`
	Whitelist    []string `json:"whitelistPaths"`
	// ProtectCfg 指针类型：不传时保持原值；传 null 或 {enabled:0} 表示改回跟随全局。
	ProtectCfg *model.SiteProtect `json:"protectCfg"`
	// 指针类型：不传时保持原值，兼容旧版前端；传空串表示改回直连。
	BehindCDN *string `json:"behindCdn"`
}

// protectCfgStr 把入参转成落库字符串。nil 不应调用本函数（调用方先判空）。
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

// normalizeBehindCDN 见 model.NormalizeBehindCDN，这里只是缩短调用点。
func normalizeBehindCDN(v string) string { return model.NormalizeBehindCDN(v) }

// SiteCreate 新增站点。
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
	c.Set(middleware.CtxAuditObj, "site")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(uint64(row.ID), 10))
	response.OK(c, toSiteDTO(row))
}

// SiteUpdate 修改站点。
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
	row.Name = in.Name
	row.Hosts = string(hosts)
	row.Upstream = in.Upstream
	row.WhitelistPath = string(wl)
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
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已保存但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "site")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, toSiteDTO(row))
}

// SiteDelete 删除站点。
func (s *Server) SiteDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	if err := s.Store.Gorm().Delete(&model.Site{}, id).Error; err != nil {
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

// SiteTest 连通性测试：请求上游根路径，返回状态码与耗时。
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

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Get(strings.TrimSuffix(row.Upstream, "/") + "/")
	if err != nil {
		response.OK(c, gin.H{"ok": false, "error": err.Error(),
			"latencyMs": time.Since(start).Milliseconds()})
		return
	}
	defer resp.Body.Close()

	response.OK(c, gin.H{
		"ok":        resp.StatusCode < 500,
		"status":    resp.StatusCode,
		"latencyMs": time.Since(start).Milliseconds(),
		"server":    resp.Header.Get("Server"),
	})
}

// validUpstream 与 config 包保持同一套规则。
func validUpstream(u string) bool {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return false
	}
	return !strings.Contains(u, ":8899")
}

// normalizeHosts 域名统一小写去空白。
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
