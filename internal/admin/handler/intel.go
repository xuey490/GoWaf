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

// IntelList 返回全部情报源列表，并附带各源在运行时的同步状态。
// 参数 c：Gin 上下文，无查询参数。
// 返回：list（headers 已打码，runtime 为该源实时统计：条数/最近同步/错误）、
// globalRanges（当前生效的全局情报区间总数）。
// 路由：GET /api/intel/sources。
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

// intelInput 是创建/修改情报源的请求体。
type intelInput struct {
	Name       string `json:"name"`       // 情报源名称，留空时归一化为「未命名情报源」
	URL        string `json:"url"`        // 拉取地址，必须以 http:// 或 https:// 开头
	Kind       string `json:"kind"`       // 清单格式：ip（纯 IP/网段列表）或 json（JSON 报文），空默认 ip
	Headers    string `json:"headers"`    // 附加请求头，形如 "Key: value" 每行一条；敏感值可能为掩码 ******
	Scope      string `json:"scope"`      // 作用范围：global（全局）或 site（单站点），空默认 global
	Site       string `json:"site"`       // scope=site 时必填的目标站点标识
	TTLSeconds int64  `json:"ttlSeconds"` // 缓存有效期（秒），即两次拉取的最小间隔
	Enabled    *int   `json:"enabled"`    // 是否启用（1 启用 0 停用）；用指针以区分「未传」与「传 0」
}

// normalize 清洗并校验请求体（字段去首尾空白、补默认名），会原地修改 in 的字段。
// 返回：空串表示校验通过；否则返回可直接展示的中文错误信息。
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
// 参数 raw：形如 "Key: value" 每行一条的原始请求头文本。
// 返回：按头名排序、敏感值替换为 ****** 后的文本；无可解析头时返回空串。
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
// 参数 oldRaw：库中原请求头文本；newRaw：前端提交的请求头文本。
// 返回：合并后的请求头文本；newRaw 无可解析头时返回空串（视为清空）。
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

// maskValue 敏感值的统一掩码文本，前端原样回传即表示「不修改该项」。
const maskValue = "******"

// isSensitiveHeader 判断请求头名是否可能携带密钥（名称含 key/auth/token/secret/cookie，不区分大小写）。
// 参数 k：请求头名称。
// 返回：true 表示该头的值需要在面板上打码。
func isSensitiveHeader(k string) bool {
	k = strings.ToLower(k)
	for _, s := range []string{"key", "auth", "token", "secret", "cookie"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// IntelCreate 新增情报源并落库。
// 参数 c：Gin 上下文，请求体为 intelInput。
// 返回：成功返回 200 与新建的 IntelSource 记录（含生成的 ID）；参数不合法返回 400。
// 路由：POST /api/intel/sources（要求 ip:ban 权限）。
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

// IntelUpdate 按 ID 修改情报源；未传的字段（如 Kind、Enabled）保持原值。
// 参数 c：Gin 上下文，路径参数 id 为情报源主键，请求体为 intelInput。
// 返回：成功返回 200 与更新后的记录；id 非法返回 400，记录不存在返回 404。
// 路由：PUT /api/intel/sources/:id（要求 ip:ban 权限）。
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

// IntelDelete 按 ID 删除情报源，并触发情报运行时重载，同步清掉内存中的区间数据。
// 参数 c：Gin 上下文，路径参数 id 为情报源主键。
// 返回：成功返回 200 与 {"ok":true}；id 非法返回 400，删除失败返回 500。
// 路由：DELETE /api/intel/sources/:id（要求 ip:ban 权限）。
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

// IntelSync 立即拉取并重建所有启用情报源的内存数据。
// 参数 c：Gin 上下文，无请求体。
// 返回：sources 参与同步的源数量、failed 同步失败数、globalRanges 同步后的全局区间总数。
// 路由：POST /api/intel/sync（要求 ip:ban 权限）。
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

// IntelStatus 返回情报与检测引擎的只读状态，供设置页/仪表盘展示。
// 参数 c：Gin 上下文，无查询参数。
// 返回：globalRanges 全局区间数、stats 各源同步统计、geo 地理库状态、
// crs 第二引擎（开关/模式/偏执级别/版本）、exceptions 例外条数。
// 路由：GET /api/intel/status。
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

// crsMode 读取 CRS 第二引擎的拦截模式。
// 参数 c：CRS 引擎指针，可为 nil。
// 返回：模式字符串（block/log）；引擎未启用时返回 "off"。
func crsMode(c *detect.CRSEngine) string {
	if c == nil {
		return "off"
	}
	return c.Mode()
}

// crsParanoia 读取 CRS 引擎的偏执级别。
// 参数 c：CRS 引擎指针，可为 nil。
// 返回：1~4 的偏执级别；引擎未启用时返回 0。
func crsParanoia(c *detect.CRSEngine) int {
	if c == nil {
		return 0
	}
	return c.Paranoia()
}

// crsVersion 读取 CRS 规则集版本号。
// 参数 c：CRS 引擎指针，可为 nil。
// 返回：版本字符串；引擎未启用时返回空串。
func crsVersion(c *detect.CRSEngine) string {
	if c == nil {
		return ""
	}
	return c.Version()
}
