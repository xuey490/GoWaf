// mcp 包内部:工具注册表与各工具的实现。
//
// 每个工具对应控制台的一个 REST 端点,路径常量与 internal/admin/router.go
// 注册的真实路由一一对应(控制台没有 /api/v1 前缀,概览/时间序列挂在
// /api/dashboard 下)。工具层只做参数校验、请求构造与查询串拼装,
// 响应解包见 client.go。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// 工具后端路径常量:与控制台真实路由对齐,改动时须同步 router.go。
const (
	pathOverview   = "/api/dashboard/overview"
	pathTimeseries = "/api/dashboard/timeseries"
	pathEvents     = "/api/events"
	pathSites      = "/api/sites"
	pathBans       = "/api/bans"
	pathRules      = "/api/rules"
	pathRuleTest   = "/api/rules/test"
	pathSettings   = "/api/settings"
)

// 列表类工具的分页取值:固定取第一页,条数按任务约定。
const (
	eventsPageSize = 20 // waf_events 每页条数
	bansPageSize   = 50 // waf_bans 每页条数
	rulesPageSize  = 50 // waf_rules 每页条数
	defaultHours   = 24 // waf_timeseries 缺省统计时长(小时)
)

// toolDef 描述一个 MCP 工具:名称、中文说明、入参 JSON Schema 与后端处理函数。
type toolDef struct {
	Name        string          `json:"name"`        // 工具名,如 waf_overview
	Description string          `json:"description"` // 中文说明,供 AI 客户端理解工具用途
	InputSchema json.RawMessage `json:"inputSchema"` // 入参 JSON Schema
	// Handler 执行工具:args 为客户端传入的 arguments(可能缺省)。
	// 返回:作为 text 内容回传的 data 原文;失败返回错误(转为 isError 结果)。
	Handler func(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error)
}

// 工具入参 JSON Schema:仅使用 type/properties/required/description 子集,
// 供 AI 客户端按字段说明生成参数。
const (
	schemaOverview = `{
  "type": "object",
  "properties": {
    "site":  {"type": "string", "description": "站点名,可选;不传则统计全部站点"},
    "range": {"type": "string", "description": "时间范围,可选,取值 15m/1h/6h/24h/7d/30d,默认 24h"}
  }
}`

	schemaTimeseries = `{
  "type": "object",
  "properties": {
    "hours": {"type": "integer", "description": "统计时长(小时),可选,默认 24;自动对齐到最近支持档位 1h/6h/24h/7d/30d"},
    "site":  {"type": "string", "description": "站点名,可选;不传则统计全部站点"}
  }
}`

	schemaEvents = `{
  "type": "object",
  "properties": {
    "site":    {"type": "string", "description": "站点名,可选"},
    "action":  {"type": "string", "description": "处置动作,可选,如 block/challenge/log/pass"},
    "keyword": {"type": "string", "description": "模糊关键词,可选,匹配路径/UA/查询串/命中规则/客户端 IP"},
    "range":   {"type": "string", "description": "时间范围,可选,取值 15m/1h/6h/24h/7d/30d;不传时控制台默认统计最近 1 小时"}
  }
}`

	schemaEventDetail = `{
  "type": "object",
  "properties": {
    "id": {"type": "integer", "description": "事件 ID,来自 waf_events 列表"}
  },
  "required": ["id"]
}`

	schemaSites = `{"type": "object", "properties": {}}`

	schemaBans = `{
  "type": "object",
  "properties": {
    "site": {"type": "string", "description": "站点名,可选,只查该站点作用域的封禁"}
  }
}`

	schemaBanIP = `{
  "type": "object",
  "properties": {
    "ip":     {"type": "string", "description": "要封禁的单个 IP,不支持网段"},
    "reason": {"type": "string", "description": "封禁原因备注"},
    "ttl":    {"type": "integer", "description": "封禁时长(秒),可选,0 或缺省表示永久"},
    "scope":  {"type": "string", "description": "作用域,可选,global 全局 / site 站点,缺省按 site 是否非空推断"},
    "site":   {"type": "string", "description": "作用域为 site 时的站点名"}
  },
  "required": ["ip"]
}`

	schemaUnbanIP = `{
  "type": "object",
  "properties": {
    "id": {"type": "integer", "description": "封禁记录 ID,来自 waf_bans 列表"}
  },
  "required": ["id"]
}`

	schemaRules = `{
  "type": "object",
  "properties": {
    "keyword":  {"type": "string", "description": "关键词,可选,模糊匹配规则 ID 与名称"},
    "category": {"type": "string", "description": "规则分类,可选,如 sqli/xss"},
    "enabled":  {"type": "string", "description": "是否启用,可选,1 只看启用 / 0 只看停用"}
  }
}`

	schemaRuleTest = `{
  "type": "object",
  "properties": {
    "method":  {"type": "string", "description": "HTTP 方法,必填,如 GET/POST"},
    "path":    {"type": "string", "description": "请求路径,必填,如 /login.php"},
    "query":   {"type": "string", "description": "查询串,可选,如 id=1"},
    "body":    {"type": "string", "description": "请求体,可选"},
    "headers": {"type": "object", "additionalProperties": {"type": "string"}, "description": "请求头键值对,可选"}
  },
  "required": ["method", "path"]
}`

	schemaSettings = `{"type": "object", "properties": {}}`
)

// toolWire 是 tools/list 返回的工具描述,只含协议字段;toolDef.Handler 是
// Go 函数,不能进入 JSON 输出,故单独定义线格式结构。
type toolWire struct {
	Name        string          `json:"name"`        // 工具名
	Description string          `json:"description"` // 中文说明
	InputSchema json.RawMessage `json:"inputSchema"` // 入参 JSON Schema
}

// wireTools 把工具注册表转换为 tools/list 的返回列表。
// 返回:全部工具的协议描述,顺序与注册表一致。
func wireTools() []toolWire {
	list := make([]toolWire, 0, len(toolRegistry))
	for _, t := range toolRegistry {
		list = append(list, toolWire{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return list
}

// toolRegistry 全部 MCP 工具,顺序即 tools/list 的返回顺序。
var toolRegistry = []*toolDef{
	{
		Name:        "waf_overview",
		Description: "获取 WAF 仪表盘总览指标:请求总量、放行/拦截/人机挑战数、5xx 数、QPS 与拦截率,可按站点与时间范围过滤。",
		InputSchema: json.RawMessage(schemaOverview),
		Handler:     handleOverview,
	},
	{
		Name:        "waf_timeseries",
		Description: "获取 WAF 流量时间序列:按时间桶聚合的统计点,默认最近 24 小时,桶宽由控制台按跨度自动选择。",
		InputSchema: json.RawMessage(schemaTimeseries),
		Handler:     handleTimeseries,
	},
	{
		Name:        "waf_events",
		Description: "分页查询安全事件(第一页 20 条):每条含时间、站点、客户端 IP、方法、路径、处置动作、评分与命中规则,支持按站点/动作/关键词/时间范围过滤。",
		InputSchema: json.RawMessage(schemaEvents),
		Handler:     handleEvents,
	},
	{
		Name:        "waf_event_detail",
		Description: "按 ID 查询单条安全事件的完整详情:查询串、UA、Referer、命中规则 ID、请求耗时等。",
		InputSchema: json.RawMessage(schemaEventDetail),
		Handler:     handleEventDetail,
	},
	{
		Name:        "waf_sites",
		Description: "列出全部受保护站点:名称、匹配域名、回源上游、启用状态与防护配置。",
		InputSchema: json.RawMessage(schemaSites),
		Handler:     handleSites,
	},
	{
		Name:        "waf_bans",
		Description: "分页查询当前生效的 IP 封禁记录(第一页 50 条):含 IP、原因、来源、作用域、命中次数与过期时间。",
		InputSchema: json.RawMessage(schemaBans),
		Handler:     handleBans,
	},
	{
		Name:        "waf_ban_ip",
		Description: "手动封禁一个 IP:可设置时长(秒,缺省永久)与原因,可选作用域 global/site;需要具备 ip:ban 权限的令牌。",
		InputSchema: json.RawMessage(schemaBanIP),
		Handler:     handleBanIP,
	},
	{
		Name:        "waf_unban_ip",
		Description: "按封禁记录 ID 解除封禁,同时释放该记录关联的内存封禁条目。",
		InputSchema: json.RawMessage(schemaUnbanIP),
		Handler:     handleUnbanIP,
	},
	{
		Name:        "waf_rules",
		Description: "分页查询检测规则(第一页 50 条):含规则 ID、名称、分类、动作与启用状态,支持关键词/分类/启用状态过滤。",
		InputSchema: json.RawMessage(schemaRules),
		Handler:     handleRules,
	},
	{
		Name:        "waf_rule_test",
		Description: "用模拟请求测试规则检测效果(不转发、不落库):method 与 path 必填,query/body/headers 可选,返回评分、动作与命中的规则。",
		InputSchema: json.RawMessage(schemaRuleTest),
		Handler:     handleRuleTest,
	},
	{
		Name:        "waf_settings",
		Description: "读取控制台设置项与检测引擎信息;SMTP 密码等敏感项已掩码回显。",
		InputSchema: json.RawMessage(schemaSettings),
		Handler:     handleSettings,
	},
}

// ===== 工具实现 =====

// banCreateBody 对齐 internal/admin/handler/network.go 中 BanCreate 的请求结构体。
type banCreateBody struct {
	IP       string `json:"ip"`       // 要封禁的单个 IP,不支持 CIDR
	Duration int64  `json:"duration"` // 封禁时长(秒),0 表示永久
	Reason   string `json:"reason"`   // 封禁原因备注
	Scope    string `json:"scope"`    // 作用域:global/site
	Site     string `json:"site"`     // 作用域为 site 时的站点名
}

// ruleTestBody 对齐 internal/detect.TestConfig 的规则测试请求体。
type ruleTestBody struct {
	Method  string            `json:"method"`  // HTTP 方法,必填
	Path    string            `json:"path"`    // 请求路径,必填
	Query   string            `json:"query"`   // 查询串,可选
	Body    string            `json:"body"`    // 请求体,可选
	Headers map[string]string `json:"headers"` // 请求头键值对,可选
}

// handleOverview 查询仪表盘总览指标。
func handleOverview(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Site  string `json:"site"`
		Range string `json:"range"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return nil, err
	}
	q := url.Values{}
	setQueryIfSet(q, "site", in.Site)
	setQueryIfSet(q, "range", in.Range)
	return cc.get(ctx, pathOverview, q)
}

// handleTimeseries 查询流量时间序列,把 hours 映射为控制台支持的 range 档位。
func handleTimeseries(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Hours int64  `json:"hours"`
		Site  string `json:"site"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return nil, err
	}
	if in.Hours <= 0 {
		in.Hours = defaultHours
	}
	q := url.Values{}
	q.Set("range", timeseriesLabel(in.Hours))
	setQueryIfSet(q, "site", in.Site)
	return cc.get(ctx, pathTimeseries, q)
}

// timeseriesLabel 把小时数就近映射为控制台 parseRange 支持的时间范围档位。
// 控制台只认 15m/1h/6h/24h/7d/30d,非法值会被回落为 1h,这里直接取最近档,
// 避免「传 48 小时结果只拿到 1 小时」的困惑。
// 参数 hours:统计时长(小时),调用方保证 >0。
// 返回:档位标识(如 "24h")。
func timeseriesLabel(hours int64) string {
	switch {
	case hours <= 1:
		return "1h"
	case hours <= 6:
		return "6h"
	case hours <= 24:
		return "24h"
	case hours <= 7*24:
		return "7d"
	default:
		return "30d"
	}
}

// handleEvents 查询安全事件列表:固定第一页,透传可选过滤参数。
func handleEvents(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Site    string `json:"site"`
		Action  string `json:"action"`
		Keyword string `json:"keyword"`
		Range   string `json:"range"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("current", "1")
	q.Set("size", strconv.Itoa(eventsPageSize))
	setQueryIfSet(q, "site", in.Site)
	setQueryIfSet(q, "action", in.Action)
	setQueryIfSet(q, "keyword", in.Keyword)
	setQueryIfSet(q, "range", in.Range)
	return cc.get(ctx, pathEvents, q)
}

// handleEventDetail 按 ID 查询单条安全事件详情。
func handleEventDetail(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	m, err := argMap(args)
	if err != nil {
		return nil, err
	}
	id, err := argID(m, "id")
	if err != nil {
		return nil, err
	}
	return cc.get(ctx, pathEvents+"/"+strconv.FormatInt(id, 10), nil)
}

// handleSites 列出全部受保护站点。
func handleSites(ctx context.Context, cc *consoleClient, _ json.RawMessage) (json.RawMessage, error) {
	return cc.get(ctx, pathSites, nil)
}

// handleBans 查询当前生效的封禁记录:控制台默认只查未释放且未过期的记录。
func handleBans(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Site string `json:"site"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("current", "1")
	q.Set("size", strconv.Itoa(bansPageSize))
	setQueryIfSet(q, "site", in.Site)
	return cc.get(ctx, pathBans, q)
}

// handleBanIP 手动封禁 IP:请求体字段对齐 BanCreate,ttl 映射为 duration(秒)。
func handleBanIP(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		IP     string `json:"ip"`
		Reason string `json:"reason"`
		TTL    int64  `json:"ttl"` // 封禁时长(秒),缺省/0 表示永久
		Scope  string `json:"scope"`
		Site   string `json:"site"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return nil, err
	}
	in.IP = strings.TrimSpace(in.IP)
	if in.IP == "" {
		return nil, fmt.Errorf("缺少必填参数 ip")
	}
	body := banCreateBody{
		IP: in.IP, Duration: in.TTL, Reason: in.Reason, Scope: in.Scope, Site: in.Site,
	}
	return cc.postJSON(ctx, pathBans, body)
}

// handleUnbanIP 按封禁记录 ID 解除封禁。
func handleUnbanIP(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	m, err := argMap(args)
	if err != nil {
		return nil, err
	}
	id, err := argID(m, "id")
	if err != nil {
		return nil, err
	}
	return cc.delete(ctx, pathBans+"/"+strconv.FormatInt(id, 10))
}

// handleRules 查询检测规则列表:固定第一页,透传可选过滤参数。
func handleRules(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Keyword  string `json:"keyword"`
		Category string `json:"category"`
		Enabled  string `json:"enabled"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("current", "1")
	q.Set("size", strconv.Itoa(rulesPageSize))
	setQueryIfSet(q, "keyword", in.Keyword)
	setQueryIfSet(q, "category", in.Category)
	setQueryIfSet(q, "enabled", in.Enabled)
	return cc.get(ctx, pathRules, q)
}

// handleRuleTest 规则测试:method 统一大写后对齐 detect.TestConfig 转发。
func handleRuleTest(ctx context.Context, cc *consoleClient, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Method  string            `json:"method"`
		Path    string            `json:"path"`
		Query   string            `json:"query"`
		Body    string            `json:"body"`
		Headers map[string]string `json:"headers"`
	}
	if err := decodeArgs(args, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Method) == "" {
		return nil, fmt.Errorf("缺少必填参数 method")
	}
	if strings.TrimSpace(in.Path) == "" {
		return nil, fmt.Errorf("缺少必填参数 path")
	}
	body := ruleTestBody{
		Method: strings.ToUpper(strings.TrimSpace(in.Method)), Path: in.Path,
		Query: in.Query, Body: in.Body, Headers: in.Headers,
	}
	return cc.postJSON(ctx, pathRuleTest, body)
}

// handleSettings 读取控制台设置项与引擎信息。
func handleSettings(ctx context.Context, cc *consoleClient, _ json.RawMessage) (json.RawMessage, error) {
	return cc.get(ctx, pathSettings, nil)
}

// ===== 参数解析辅助 =====

// decodeArgs 把工具入参解码到目标结构体;空参数或 null 视为零值不报错。
// 参数 args:原始 arguments;dst:目标结构体指针。
// 返回:参数不是对象或字段类型不符时返回错误。
func decodeArgs(args json.RawMessage, dst any) error {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, dst); err != nil {
		return fmt.Errorf("arguments 解析失败: %w", err)
	}
	return nil
}

// argMap 把 arguments 解析成键到原始 JSON 的映射,空参数返回空映射。
// 参数 args:原始 arguments。
// 返回:参数映射;不是 JSON 对象时返回错误。
func argMap(args json.RawMessage) (map[string]json.RawMessage, error) {
	m := make(map[string]json.RawMessage, 4)
	if len(args) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(args, &m); err != nil {
		return nil, fmt.Errorf("arguments 必须是 JSON 对象: %w", err)
	}
	return m, nil
}

// argID 从参数映射取正整数 ID,兼容 JSON 数字与数字字符串两种写法
// (客户端偶尔会把整数 ID 传成字符串)。
// 参数 m:参数映射;key:参数名。
// 返回:解析出的 ID;缺失、非整数或 <=0 时返回错误。
func argID(m map[string]json.RawMessage, key string) (int64, error) {
	raw, ok := m[key]
	if !ok || len(raw) == 0 {
		return 0, fmt.Errorf("缺少必填参数 %s", key)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, fmt.Errorf("参数 %s 解析失败: %w", key, err)
	}
	switch t := v.(type) {
	case float64:
		if t != math.Trunc(t) || t <= 0 {
			return 0, fmt.Errorf("参数 %s 必须是正整数", key)
		}
		return int64(t), nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("参数 %s 必须是正整数", key)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("参数 %s 必须是正整数", key)
	}
}

// setQueryIfSet 值非空时写入查询串,避免把空参数发给控制台。
func setQueryIfSet(q url.Values, key, val string) {
	if val != "" {
		q.Set(key, val)
	}
}
