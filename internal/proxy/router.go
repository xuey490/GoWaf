package proxy

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"

	"gowaf/internal/store/model"
)

// Target 是站点的一条回源上游（双链路/多线路）。
//
// 同一站点可配置多条：Priority 数字小的优先使用，同优先级按 Weight 加权随机；
// 当前目标连接失败时自动切换到下一条，全部失败才返回 502。
// Healthy 由后台健康检查维护；首次加载默认健康，避免启动瞬间把所有目标判死。
type Target struct {
	URL      *url.URL    // 解析后的上游地址
	Raw      string      // 上游地址原文，日志与缓存键使用
	Line     string      // 线路标签：telecom/unicom/mobile/bgp
	Weight   int         // 权重，越大越容易被选中
	Priority int         // 优先级，数字小的先使用
	Healthy  atomic.Bool // 健康检查结果；默认 true
}

// Route 是一个站点路由，由 BuildRouter 从 sites 表记录编译而来；
// 数据面只读，字段都是预解析后的生效值，请求路径上不再查库查设置。
type Route struct {
	SiteID         uint      // 所属站点 ID
	SiteName       string    // 站点名称（日志与界面展示用）
	Hosts          []string  // 站点绑定的域名列表，可含 *.example.com 泛域名
	Upstream       *url.URL  // 首选上游地址，保留以兼容单上游逻辑
	UpstreamRaw    string    // 首选上游地址原文，兼作日志/事件展示
	Targets        []*Target // 全部回源目标（含首选）；单上游时长度为 1
	DetectBody     bool      // 是否检测请求体内容
	MaxBodyBytes   int       // 请求体大小上限（字节），构建时 <=0 会回填默认值
	WhitelistPath  []string  // 免检测路径白名单，支持 * 前缀通配
	RateLimitRPS   int       // 站点级限流：每秒请求数，0 表示不启用
	RateLimitBurst int       // 站点级限流：令牌桶容量
	// BehindCDN 站点是否在 CDN 回源之后：'' 直连 ｜ 'cloudflare'。
	BehindCDN string
	// Guard 站点启用自定义防护时为非 nil，阈值/模式已解析为生效值；
	// nil 表示跟随全局。同时它也是"限流计数独立"的标记。
	Guard *SiteGuard
}

// SiteGuard 站点级防护生效值（已与全局合并，数据面直接用，不再查设置）。
type SiteGuard struct {
	Mode           string // observe/protect
	BlockScore     int    // 达到该评分直接封禁
	ChallengeScore int    // 达到该评分下发人机验证
	IPRPS          int    // 单 IP 每秒请求数阈值，0 表示不启用
	IPBurst        int    // 单 IP 限流令牌桶容量
	IPPerMinute    int    // 单 IP 每分钟请求数阈值
}

// Router 按 Host 匹配站点。只读结构；站点配置变更时整体重建并原子替换。
type Router struct {
	exact    map[string]*Route // 精确域名 -> 路由（key 已过 normalizeHost 规整）
	wildcard []*Route          // 泛域名路由，构建时按域名数量降序排，越具体越靠前
	fallback *Route            // 默认站点：Host 未命中任何条目时的兜底
}

// rateLimitConfig 对应 sites.rate_limit 列。
type rateLimitConfig struct {
	RPS   int `json:"rps"`   // 每秒请求数
	Burst int `json:"burst"` // 令牌桶容量
}

// BuildRouter 从数据库记录构建只读路由表（单上游兼容入口）。
// 参数 sites：sites 表全部记录，禁用站点会被跳过。
// 参数 guards：站点 ID -> 站点级防护生效值（未启用自定义防护的站点不在 map 中）。
// 返回 (*Router, error)：任一启用站点的上游地址或 hosts 非法时报错，构建中断。
func BuildRouter(sites []model.Site, guards map[uint]*SiteGuard) (*Router, error) {
	return BuildRouterWithUpstreams(sites, guards, nil)
}

// BuildRouterWithUpstreams 构建支持多上游的路由表。
// 参数 extra：站点 ID -> 额外的回源目标；为空时行为与单上游完全一致。
// 主 upstream 始终作为第一条目标，额外目标按优先级/权重参与调度。
func BuildRouterWithUpstreams(sites []model.Site, guards map[uint]*SiteGuard, extra map[uint][]model.SiteUpstream) (*Router, error) {
	r := &Router{exact: make(map[string]*Route, len(sites))}

	for _, s := range sites {
		if s.Enabled == 0 {
			continue // 禁用站点不进路由表
		}
		u, err := url.Parse(strings.TrimSpace(s.Upstream))
		if err != nil {
			return nil, fmt.Errorf("站点 %s 上游地址非法: %w", s.Name, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("站点 %s 上游地址非法: 缺少 scheme/host", s.Name)
		}

		var hosts []string
		if s.Hosts != "" {
			if err := json.Unmarshal([]byte(s.Hosts), &hosts); err != nil {
				return nil, fmt.Errorf("站点 %s hosts 解析失败: %w", s.Name, err)
			}
		}
		var whitelist []string
		if s.WhitelistPath != "" {
			// 白名单 JSON 损坏时不阻断启动，按空列表处理
			_ = json.Unmarshal([]byte(s.WhitelistPath), &whitelist)
		}
		var rl rateLimitConfig
		if s.RateLimit != "" {
			// 限流 JSON 损坏时按 0（不启用）处理，同样不阻断启动
			_ = json.Unmarshal([]byte(s.RateLimit), &rl)
		}

		primary := &Target{URL: u, Raw: s.Upstream, Line: "bgp", Weight: 1, Priority: 1}
		primary.Healthy.Store(true)
		targets := []*Target{primary}
		seen := map[string]struct{}{normalizeTarget(s.Upstream): {}}
		for _, up := range extra[s.ID] {
			if up.Enabled == 0 {
				continue
			}
			raw := strings.TrimSpace(up.Addr)
			if raw == "" {
				continue
			}
			tu, err := url.Parse(raw)
			if err != nil || tu.Scheme == "" || tu.Host == "" {
				return nil, fmt.Errorf("站点 %s 额外上游地址非法: %s", s.Name, raw)
			}
			key := normalizeTarget(raw)
			if _, ok := seen[key]; ok {
				continue // 主上游重复配置时不再叠加
			}
			seen[key] = struct{}{}
			t := &Target{
				URL: tu, Raw: raw, Line: strings.TrimSpace(up.Line),
				Weight: up.Weight, Priority: up.Priority,
			}
			if t.Line == "" {
				t.Line = "bgp"
			}
			if t.Weight <= 0 {
				t.Weight = 1
			}
			if t.Priority <= 0 {
				t.Priority = 100
			}
			t.Healthy.Store(true)
			targets = append(targets, t)
		}

		route := &Route{
			SiteID:         s.ID,
			SiteName:       s.Name,
			Hosts:          hosts,
			Upstream:       u,
			UpstreamRaw:    s.Upstream,
			Targets:        targets,
			DetectBody:     s.DetectBody == 1,
			MaxBodyBytes:   s.MaxBodyBytes,
			WhitelistPath:  whitelist,
			RateLimitRPS:   rl.RPS,
			RateLimitBurst: rl.Burst,
			BehindCDN:      strings.ToLower(strings.TrimSpace(s.BehindCDN)),
			Guard:          guards[s.ID],
		}
		if route.MaxBodyBytes <= 0 {
			route.MaxBodyBytes = 262144 // 未配置时默认 256 KiB
		}

		if len(hosts) == 0 {
			// 未绑定域名的站点不能精确匹配，只能作默认站点兜底；先到先得
			if r.fallback == nil {
				r.fallback = route
			}
			continue
		}
		for _, h := range hosts {
			h = normalizeHost(h)
			if strings.HasPrefix(h, "*.") {
				r.wildcard = append(r.wildcard, route)
				continue
			}
			r.exact[h] = route
		}
		// 绑定了域名的站点也参与兜底候选（优先级低于未绑定域名的站点）
		if r.fallback == nil {
			r.fallback = route
		}
	}

	// 泛域名路由按绑定域名数量降序做稳定排序：绑得越多视为越具体，优先匹配
	sort.SliceStable(r.wildcard, func(i, j int) bool {
		return len(r.wildcard[i].Hosts) > len(r.wildcard[j].Hosts)
	})
	return r, nil
}

// normalizeTarget 规整上游地址用于去重：转小写、去首尾空白与末尾斜杠。
// 参数 raw：上游地址原文。返回 string：规整后的比较键。
func normalizeTarget(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), "/")
}

// normalizeHost 规整域名用于匹配：去首尾空白、转小写、去掉 host:port 形式的端口。
// 参数 h：原始 Host 头或配置里的域名。
// 返回 string：规整后的域名；IPv6 字面量（冒号不止一个）不做剥端口处理。
func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if i := strings.LastIndexByte(h, ':'); i > 0 && !strings.Contains(h[i:], "]") {
		// 只处理 host:port 形式，避免误伤 IPv6
		if !strings.Contains(h[:i], ":") {
			h = h[:i]
		}
	}
	return h
}

// Match 按 Host 找站点。
// 参数 host：请求的 Host 头（可带端口，内部规整后匹配）。
// 返回 *Route：精确匹配优先，其次遍历泛域名路由，仍未命中回落默认站点；
// 接收者或路由表为空时返回 nil。
func (r *Router) Match(host string) *Route {
	if r == nil {
		return nil
	}
	h := normalizeHost(host)
	if rt, ok := r.exact[h]; ok {
		return rt
	}
	for _, rt := range r.wildcard {
		// 泛域名匹配：剥掉模式里的 * 取后缀，请求域名以后缀结尾即命中
		for _, pattern := range rt.Hosts {
			p := normalizeHost(pattern)
			suffix := strings.TrimPrefix(p, "*")
			if strings.HasSuffix(h, suffix) {
				return rt
			}
		}
	}
	return r.fallback
}

// IsPathWhitelisted 判断路径是否在站点白名单内（支持 * 通配）。
// 参数 path：请求路径。
// 返回 bool：以 * 结尾的条目按前缀匹配，其余要求完全相等，命中任一条即 true。
func (rt *Route) IsPathWhitelisted(path string) bool {
	for _, p := range rt.WhitelistPath {
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(p, "*")) {
				return true
			}
			continue
		}
		if path == p {
			return true
		}
	}
	return false
}

// Count 返回路由表中的站点数量，按 SiteID 去重（同一站点可绑定多个域名，
// exact / wildcard / fallback 可能指向同一个 *Route）。
// 无参数；返回 int：去重后的站点数；nil 接收者返回 0。
func (r *Router) Count() int {
	if r == nil {
		return 0
	}
	seen := map[uint]struct{}{}
	for _, rt := range r.exact {
		seen[rt.SiteID] = struct{}{}
	}
	for _, rt := range r.wildcard {
		seen[rt.SiteID] = struct{}{}
	}
	if r.fallback != nil {
		seen[r.fallback.SiteID] = struct{}{}
	}
	return len(seen)
}

// Sites 返回全部站点路由，按 SiteID 去重（同一站点的多个域名指向同一个
// *Route，只返回一份）。遍历场景（防篡改基线刷新等）使用，顺序不保证稳定。
// 无参数；返回 []*Route：去重后的站点路由；nil 接收者返回 nil。
func (r *Router) Sites() []*Route {
	if r == nil {
		return nil
	}
	seen := map[uint]struct{}{}
	out := make([]*Route, 0, len(r.exact))
	collect := func(rt *Route) {
		if rt == nil {
			return
		}
		if _, ok := seen[rt.SiteID]; ok {
			return
		}
		seen[rt.SiteID] = struct{}{}
		out = append(out, rt)
	}
	for _, rt := range r.exact {
		collect(rt)
	}
	for _, rt := range r.wildcard {
		collect(rt)
	}
	collect(r.fallback)
	return out
}

// CDNSites 返回标记了 CDN 回源的站点名，按站点去重并排序。
//
// 一个站点可能命中多个域名（exact / wildcard / fallback 指向同一个 *Route），
// 所以必须按 SiteID 去重，否则自检提示会重复。
// 无参数；返回 []string：站点名列表（升序），没有 CDN 站点时为空。
func (r *Router) CDNSites() []string {
	if r == nil {
		return nil
	}
	seen := map[uint]struct{}{}
	out := make([]string, 0, 2)
	collect := func(rt *Route) {
		if rt == nil || rt.BehindCDN == "" {
			return
		}
		if _, ok := seen[rt.SiteID]; ok {
			return
		}
		seen[rt.SiteID] = struct{}{}
		out = append(out, rt.SiteName)
	}
	for _, rt := range r.exact {
		collect(rt)
	}
	for _, rt := range r.wildcard {
		collect(rt)
	}
	collect(r.fallback)
	sort.Strings(out)
	return out
}
