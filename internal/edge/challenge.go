package edge

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"gowaf/internal/proxy"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
)

// challengeReason 判断本次请求是否需要先过一次人机校验，返回触发原因。
//
// 只对「页面导航」生效（GET 的顶层文档请求）。原因是校验依赖浏览器执行 JS：
// 对 API / XHR / 服务间调用下发校验页，等价于直接封禁且对方无法自证——
// 这类流量继续交给硬限流返回 429，语义更清楚，也不会把正常前端轮询打断。
//
// 判定顺序：跨站来路（纯计算，无状态）→ 请求频率（会消耗软阈值令牌）。
// 参数 r：客户端请求；参数 route：命中的站点路由；参数 ip：客户端 IP。
// 返回 string："referer"（跨站来路）/ "rate"（频率超软阈值）/ ""（不需要校验）。
func (h *Handler) challengeReason(r *http.Request, route *proxy.Route, ip netip.Addr) string {
	if !isDocumentNavigation(r) {
		return ""
	}
	if h.refererIsForeign(r, route) {
		return "referer"
	}
	if h.rateOverSoftLimit(ip) {
		return "rate"
	}
	return ""
}

// isDocumentNavigation 判断是否为浏览器顶层页面导航。
// 优先用 Sec-Fetch-Dest（现代浏览器都发），缺失时退回 Accept 判断。
// 参数 r：客户端请求；返回 bool：是页面导航时为 true。
func isDocumentNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" {
		return dest == "document"
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

// refererIsForeign 判断请求是否来自「陌生的外部站点」。
//
// 语义与 Cloudflare 的导流判定一致：从别的网站点进来（Referer 域名不属于本站）
// 先自动校验一次，挡掉广告链、群发短信链接带来的脚本化流量。
// 直接访问 / 书签 / 隐私模式下没有 Referer，这类请求不挑战——否则会误伤大量正常用户。
// 参数 r：客户端请求；参数 route：命中的站点路由。
// 返回 bool：来路是外站且不在允许清单内时为 true。
func (h *Handler) refererIsForeign(r *http.Request, route *proxy.Route) bool {
	if !h.rt.SettingBool(runtime.KeyChallengeReferer, h.rt.Cfg.Guard.RefererChallengeEnabled()) {
		return false
	}
	ref := r.Referer()
	if ref == "" {
		return false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	if host == requestHost(r) || hostMatchesAny(host, route.Hosts) {
		return false
	}
	allow := h.rt.Setting(runtime.KeyChallengeRefererAllow, h.rt.Cfg.Guard.ChallengeRefererAllow)
	if hostInAllowList(host, allow) {
		return false
	}
	return true
}

// rateOverSoftLimit 判断该 IP 是否超过了人机校验的软阈值。
//
// 软阈值低于硬限流：超了先验一次人，验过继续访问；硬限流仍然是 429。
// 两个阈值都配成 0 时视为关闭该触发方式。
// 参数 ip：客户端 IP；返回 bool：需要下发校验时为 true。
func (h *Handler) rateOverSoftLimit(ip netip.Addr) bool {
	rps := h.rt.SettingInt(runtime.KeyChallengeRPS, h.rt.Cfg.Guard.ChallengeRPS)
	perMin := h.rt.SettingInt(runtime.KeyChallengePerMinute, h.rt.Cfg.Guard.ChallengePerMinute)
	if rps <= 0 && perMin <= 0 {
		return false
	}
	l := h.rt.ChallengeLimiter()
	if l == nil {
		return false
	}
	ok, _ := l.Allow(ip.String(), store.NowMS())
	return !ok
}

// requestHost 取请求 Host 头里的主机名（去掉端口、转小写）。
// 参数 r：客户端请求；返回 string：主机名，Host 为空时返回空串。
func requestHost(r *http.Request) string {
	h := strings.ToLower(strings.TrimSpace(r.Host))
	if hp, _, err := net.SplitHostPort(h); err == nil {
		return hp
	}
	return strings.Trim(h, "[]")
}

// hostMatchesAny 判断主机名是否命中站点绑定的域名列表（支持 *.example.com 泛域名）。
// 参数 host：待匹配主机名（已转小写）；参数 patterns：站点 hosts 列表。
// 返回 bool：任一模式命中时为 true。
func hostMatchesAny(host string, patterns []string) bool {
	for _, p := range patterns {
		if hostMatchesPattern(host, strings.ToLower(strings.TrimSpace(p))) {
			return true
		}
	}
	return false
}

// hostInAllowList 判断主机名是否在逗号分隔的允许清单内。
// 参数 host：待匹配主机名（小写）；参数 list：形如 "www.baidu.com,*.example.com" 的清单。
// 返回 bool：命中任一模式时为 true。
func hostInAllowList(host, list string) bool {
	for _, item := range strings.Split(list, ",") {
		if hostMatchesPattern(host, strings.ToLower(strings.TrimSpace(item))) {
			return true
		}
	}
	return false
}

// hostMatchesPattern 单条模式匹配：精确相等，或 *.suffix 形式的后缀匹配。
// 参数 host：小写主机名；参数 pattern：小写模式。
// 返回 bool：匹配成功时为 true。
func hostMatchesPattern(host, pattern string) bool {
	if host == "" || pattern == "" {
		return false
	}
	if host == pattern {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix) && len(host) > len(suffix)
	}
	return false
}
