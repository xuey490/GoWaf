// Package proxy 负责站点路由与反向代理。
//
// 关键设计：不做任何 WAF 检测，只负责"把请求送到正确的上游"。
// 检测在 detect 包，两者通过 app 层串联，保持数据面依赖单一。
package proxy

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gowaf/internal/store/model"
)

// Route 是一个站点路由。
type Route struct {
	SiteID         uint
	SiteName       string
	Hosts          []string
	Upstream       *url.URL
	UpstreamRaw    string
	DetectBody     bool
	MaxBodyBytes   int
	WhitelistPath  []string
	RateLimitRPS   int
	RateLimitBurst int
	// BehindCDN 站点是否在 CDN 回源之后：'' 直连 ｜ 'cloudflare'。
	BehindCDN string
	// Guard 站点启用自定义防护时为非 nil，阈值/模式已解析为生效值；
	// nil 表示跟随全局。同时它也是"限流计数独立"的标记。
	Guard *SiteGuard
}

// SiteGuard 站点级防护生效值（已与全局合并，数据面直接用，不再查设置）。
type SiteGuard struct {
	Mode           string // observe/protect
	BlockScore     int
	ChallengeScore int
	IPRPS          int
	IPBurst        int
	IPPerMinute    int
}

// Router 按 Host 匹配站点。
type Router struct {
	exact    map[string]*Route
	wildcard []*Route
	fallback *Route
}

// rateLimitConfig 对应 sites.rate_limit 列。
type rateLimitConfig struct {
	RPS   int `json:"rps"`
	Burst int `json:"burst"`
}

// BuildRouter 从数据库记录构建路由表。
func BuildRouter(sites []model.Site, guards map[uint]*SiteGuard) (*Router, error) {
	r := &Router{exact: make(map[string]*Route, len(sites))}

	for _, s := range sites {
		if s.Enabled == 0 {
			continue
		}
		u, err := url.Parse(s.Upstream)
		if err != nil {
			return nil, fmt.Errorf("站点 %s 上游地址非法: %w", s.Name, err)
		}

		var hosts []string
		if s.Hosts != "" {
			if err := json.Unmarshal([]byte(s.Hosts), &hosts); err != nil {
				return nil, fmt.Errorf("站点 %s hosts 解析失败: %w", s.Name, err)
			}
		}
		var whitelist []string
		if s.WhitelistPath != "" {
			_ = json.Unmarshal([]byte(s.WhitelistPath), &whitelist)
		}
		var rl rateLimitConfig
		if s.RateLimit != "" {
			_ = json.Unmarshal([]byte(s.RateLimit), &rl)
		}

		route := &Route{
			SiteID:         s.ID,
			SiteName:       s.Name,
			Hosts:          hosts,
			Upstream:       u,
			UpstreamRaw:    s.Upstream,
			DetectBody:     s.DetectBody == 1,
			MaxBodyBytes:   s.MaxBodyBytes,
			WhitelistPath:  whitelist,
			RateLimitRPS:   rl.RPS,
			RateLimitBurst: rl.Burst,
			BehindCDN:      strings.ToLower(strings.TrimSpace(s.BehindCDN)),
			Guard:          guards[s.ID],
		}
		if route.MaxBodyBytes <= 0 {
			route.MaxBodyBytes = 262144
		}

		if len(hosts) == 0 {
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
		if r.fallback == nil {
			r.fallback = route
		}
	}

	// 通配符按后缀长度排序，最长优先
	sort.SliceStable(r.wildcard, func(i, j int) bool {
		return len(r.wildcard[i].Hosts) > len(r.wildcard[j].Hosts)
	})
	return r, nil
}

// normalizeHost 去掉端口、转小写。
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

// Match 按 Host 找站点；找不到时回落到默认站点（第一个可用站点）。
func (r *Router) Match(host string) *Route {
	if r == nil {
		return nil
	}
	h := normalizeHost(host)
	if rt, ok := r.exact[h]; ok {
		return rt
	}
	for _, rt := range r.wildcard {
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

// Count 返回站点数量。
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

// CDNSites 返回标记了 CDN 回源的站点名，按站点去重并排序。
//
// 一个站点可能命中多个域名（exact / wildcard / fallback 指向同一个 *Route），
// 所以必须按 SiteID 去重，否则自检提示会重复。
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
