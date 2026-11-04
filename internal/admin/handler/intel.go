package handler

import (
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/detect"
	"gowaf/internal/intel"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

/* ====================================================== P3 威胁情报 */

// IntelList 情报源列表 + 运行时状态。
func (s *Server) IntelList(c *gin.Context) {
	var rows []model.IntelSource
	if err := s.Store.Gorm().Order("id ASC").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	stats := map[uint]gin.H{}
	if s.RT.Intel != nil {
		for _, st := range s.RT.Intel.Stats() {
			stats[st.ID] = gin.H{
				"count": st.Count, "lastSync": st.LastSync, "error": st.Error,
			}
		}
	}
	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		list = append(list, gin.H{
			"id": r.ID, "name": r.Name, "url": r.URL, "kind": r.Kind,
			"headers": maskHeaders(r.Headers),
			"scope":   r.Scope, "site": r.Site, "ttlSeconds": r.TTLSeconds,
			"enabled": r.Enabled, "lastSyncAt": r.LastSyncAt, "lastCount": r.LastCount,
			"lastError": r.LastError, "runtime": stats[r.ID],
		})
	}
	globalCount := 0
	if s.RT.Intel != nil {
		globalCount = s.RT.Intel.GlobalCount()
	}
	response.OK(c, gin.H{"list": list, "globalRanges": globalCount})
}

type intelInput struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Kind       string `json:"kind"`
	Headers    string `json:"headers"`
	Scope      string `json:"scope"`
	Site       string `json:"site"`
	TTLSeconds int64  `json:"ttlSeconds"`
	Enabled    *int   `json:"enabled"`
}

func (in *intelInput) normalize() string {
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimSpace(in.URL)
	in.Site = strings.TrimSpace(in.Site)
	in.Headers = strings.TrimSpace(in.Headers)
	if in.Name == "" {
		in.Name = "未命名情报源"
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		return "url 必须以 http:// 或 https:// 开头"
	}
	if in.Kind != "" && in.Kind != "ip" && in.Kind != "json" {
		return "格式只能是 ip 或 json"
	}
	if in.Scope != "" && in.Scope != "global" && in.Scope != "site" {
		return "scope 只能是 global 或 site"
	}
	if in.Scope == "site" && in.Site == "" {
		return "scope=site 时必须指定站点"
	}
	return ""
}

// maskHeaders 把请求头里的密钥打码后再返回给前端，避免面板上明文展示 API Key。
func maskHeaders(raw string) string {
	h := intel.ParseHeaders(raw)
	if len(h) == 0 {
		return ""
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := h[k]
		if isSensitiveHeader(k) && v != "" {
			v = maskValue
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// mergeMaskedHeaders 提交时把仍是掩码的值替换回库里原值，实现「不重填就不改」。
func mergeMaskedHeaders(oldRaw, newRaw string) string {
	newH := intel.ParseHeaders(newRaw)
	if len(newH) == 0 {
		return ""
	}
	oldH := intel.ParseHeaders(oldRaw)
	keys := make([]string, 0, len(newH))
	for k := range newH {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := newH[k]
		if v == maskValue {
			if old, ok := oldH[k]; ok {
				v = old
			} else {
				continue
			}
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

const maskValue = "******"

func isSensitiveHeader(k string) bool {
	k = strings.ToLower(k)
	for _, s := range []string{"key", "auth", "token", "secret", "cookie"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// IntelCreate 新增情报源。
func (s *Server) IntelCreate(c *gin.Context) {
	var in intelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if msg := in.normalize(); msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if in.Scope == "" {
		in.Scope = "global"
	}
	enabled := 1
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	kind := in.Kind
	if kind == "" {
		kind = "ip"
	}
	row := model.IntelSource{
		Name: in.Name, URL: in.URL, Kind: kind, Headers: mergeMaskedHeaders("", in.Headers),
		Scope: in.Scope, Site: in.Site,
		TTLSeconds: in.TTLSeconds, Enabled: enabled, CreatedAt: store.NowMS(),
	}
	if err := s.Store.Gorm().Create(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "intel_source")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(uint64(row.ID), 10))
	response.OK(c, row)
}

// IntelUpdate 修改情报源。
func (s *Server) IntelUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var row model.IntelSource
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		response.NotFound(c, "情报源不存在")
		return
	}
	var in intelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if msg := in.normalize(); msg != "" {
		response.BadRequest(c, msg)
		return
	}
	row.Name, row.URL, row.Site = in.Name, in.URL, in.Site
	row.Scope = in.Scope
	if row.Scope == "" {
		row.Scope = "global"
	}
	if in.Kind != "" {
		row.Kind = in.Kind
	}
	// 前端拿到的是打码后的头；没改就是 ******，此时沿用库里的真实值。
	row.Headers = mergeMaskedHeaders(row.Headers, in.Headers)
	row.TTLSeconds = in.TTLSeconds
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if err := s.Store.Gorm().Save(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "intel_source")
	c.Set(middleware.CtxAuditID, c.Param("id"))
	response.OK(c, row)
}

// IntelDelete 删除情报源（同时清掉它在内存里的数据）。
func (s *Server) IntelDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	if err := s.Store.Gorm().Delete(&model.IntelSource{}, id).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.RT.ReloadIntel(c.Request.Context())
	c.Set(middleware.CtxAuditObj, "intel_source")
	c.Set(middleware.CtxAuditID, c.Param("id"))
	response.OK(c, gin.H{"ok": true})
}

// IntelSync 立即同步所有启用的情报源。
func (s *Server) IntelSync(c *gin.Context) {
	res := s.RT.ReloadIntel(c.Request.Context())
	failed := 0
	for _, r := range res {
		if r.Error != "" {
			failed++
		}
	}
	response.OK(c, gin.H{
		"sources": len(res), "failed": failed,
		"globalRanges": s.RT.Intel.GlobalCount(),
	})
}

// IntelStatus 只读状态，供设置页/仪表盘展示。
func (s *Server) IntelStatus(c *gin.Context) {
	stats := s.RT.Intel.Stats()
	response.OK(c, gin.H{
		"globalRanges": s.RT.Intel.GlobalCount(),
		"stats":        stats,
		"geo": gin.H{
			"ranges": s.RT.Geo.Size(),
			"ready":  s.RT.Geo.Ready(),
		},
		"crs": gin.H{
			"enabled":  s.RT.CRS() != nil,
			"mode":     crsMode(s.RT.CRS()),
			"paranoia": crsParanoia(s.RT.CRS()),
			"version":  crsVersion(s.RT.CRS()),
		},
		"exceptions": s.RT.Engine.ExceptionCount(),
	})
}

func crsMode(c *detect.CRSEngine) string {
	if c == nil {
		return "off"
	}
	return c.Mode()
}

func crsParanoia(c *detect.CRSEngine) int {
	if c == nil {
		return 0
	}
	return c.Paranoia()
}

func crsVersion(c *detect.CRSEngine) string {
	if c == nil {
		return ""
	}
	return c.Version()
}
