package runtime

import (
	"log"
	"net"
	"net/url"
	"strconv"

	"gowaf/internal/store/model"
)

// dropSelfLoopSites 过滤掉上游指向 WAF 自身监听端口的站点。
//
// 典型事故：控制台里把某个站点的上游填成 http://127.0.0.1:80 ——
// WAF 收到请求 → 转给 80 端口的自己 → 再次匹配兜底路由 → 无限递归，
// 每个外网扫描请求都会放大成一条自环链，几分钟内把内存和临时端口耗光
// （2026-11-03 生产事故：兜底站点 upstream=127.0.0.1:80，配合全网扫描
// 直接把 4GB 内存的机器拖到 OOM 边缘）。
//
// 这类配置错误必须在这一层拒绝，而不是等绑定/转发时才暴露：
// 运行时无法区分「故意」与「手滑」，一律按事故处理——丢弃该路由并打日志。
func dropSelfLoopSites(sites []model.Site, listenPorts map[string]struct{}) []model.Site {
	if len(listenPorts) == 0 {
		return sites
	}
	out := sites[:0:0]
	for _, s := range sites {
		if upstreamHitsPorts(s.Upstream, listenPorts) {
			log.Printf("[runtime] 配置风险 self_loop：站点 %s 的上游 %s 指向 WAF 自身监听端口，已拒绝加载（会造成无限代理自环）",
				s.Name, s.Upstream)
			continue
		}
		out = append(out, s)
	}
	return out
}

// upstreamHitsPorts 判断上游 URL 的端口是否命中给定端口集合。
func upstreamHitsPorts(raw string, ports map[string]struct{}) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(ensureScheme(raw))
	if err != nil {
		return false
	}
	if u.Port() == "" {
		// 无端口的 http/https 上游使用默认端口
		switch u.Scheme {
		case "https":
			return has(ports, "443")
		case "http":
			return has(ports, "80")
		}
		return false
	}
	return has(ports, u.Port())
}

func has(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}

// ensureScheme 补全无 scheme 的上游（如 "127.0.0.1:80" 会被 url.Parse
// 误认为 scheme），统一按 http 处理后再解析。
func ensureScheme(raw string) string {
	if len(raw) > 0 && !(len(raw) > 7 && raw[:7] == "http://") && !(len(raw) > 8 && raw[:8] == "https://") {
		return "http://" + raw
	}
	return raw
}

// selfListenPorts 从配置的监听地址里提取端口集合。
func selfListenPorts(listens ...string) map[string]struct{} {
	ports := make(map[string]struct{}, len(listens))
	for _, l := range listens {
		if l == "" {
			continue
		}
		_, port, err := net.SplitHostPort(l)
		if err != nil || port == "" {
			continue
		}
		if _, err := strconv.Atoi(port); err != nil {
			continue
		}
		ports[port] = struct{}{}
	}
	return ports
}
