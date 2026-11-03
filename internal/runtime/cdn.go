package runtime

import (
	"fmt"
	"strings"

	"gowaf/internal/guard"
)

// CDNWarning 是一条配置风险提示，控制台顶部横幅与启动日志共用同一份数据。
type CDNWarning struct {
	Code    string `json:"code"`
	Site    string `json:"site,omitempty"`
	Message string `json:"message"`
}

// WarnCDNTrustMissing 站点标记了 CDN 回源，但 trusted_proxies 没有覆盖回源网段。
const WarnCDNTrustMissing = "cdn_trust_missing"

// CDNWarnings 检查「站点标记的 CDN」与「trusted_proxies」是否一致。
//
// 这是混合站点部署最容易踩的坑：橙云站点的连接对端是 CF 节点，
// 若 trusted_proxies 里没有 CF 回源网段，clientIP 会退化成节点 IP。
// 一个节点背后是成千上万真实访客，限流与自动封禁会一次误伤一大片，
// 而且 bans 表里写进去的是 Cloudflare 的地址，排查时极具误导性。
//
// 只在「标记了 CDN 但没信任」时报错；反过来（信任了但没标记）是无害的，
// 因为取 IP 靠的是对端网段判定，不依赖这个标记，所以不产生噪音提示。
func (r *Runtime) CDNWarnings() []CDNWarning {
	names := r.Router().CDNSites()
	if len(names) == 0 {
		return []CDNWarning{}
	}
	missing := guard.MissingCloudflare(guard.ParseTrusted(r.Cfg.Server.TrustedProxies))
	if len(missing) == 0 {
		return []CDNWarning{}
	}
	return []CDNWarning{{
		Code: WarnCDNTrustMissing,
		Site: strings.Join(names, ","),
		Message: fmt.Sprintf(
			"站点 %s 标记为 Cloudflare 回源，但 server.trusted_proxies 未覆盖 %d 个 Cloudflare 网段（如 %s）。"+
				"当前会把 CF 节点 IP 当作访客 IP，限流与自动封禁可能一次误伤该节点后的全部用户。"+
				"请执行 install.sh cloudflare on（或 cloudflare update）后重启服务。",
			strings.Join(names, "、"), len(missing), strings.Join(headN(missing, 2), "、")),
	}}
}

// headN 取前 n 个元素，避免把 20 多个网段全塞进一条提示里。
func headN(list []string, n int) []string {
	if len(list) <= n {
		return list
	}
	return list[:n]
}
