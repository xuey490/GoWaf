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
// 参数 sites：本次 Reload 读出的启用站点；参数 listenPorts：WAF 自身监听端口集合（键为十进制端口字符串）。
// 返回 []model.Site：过滤后的站点列表；被剔除的站点逐个打日志说明原因。
func dropSelfLoopSites(sites []model.Site, listenPorts map[string]struct{}) []model.Site {
	if len(listenPorts) == 0 {
		return sites
	}
	// 三索引切片：新建底层数组，避免原地覆盖调用方切片的内容
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

// upstreamHitsPorts 判断上游地址的端口是否命中给定端口集合。
// 参数 raw：上游地址（可以不带 scheme）；参数 ports：WAF 自身监听端口集合。
// 返回 bool：true 表示该上游会连回 WAF 自身（自环），需剔除；
// 地址为空或解析失败时按不命中处理，交给后续绑定/转发阶段兜底。
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

// has 判断键是否存在于集合。
// 参数 m：目标集合；参数 k：待查键。
// 返回 bool：存在为 true。
func has(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}

// ensureScheme 补全无 scheme 的上游地址。
// 如 "127.0.0.1:80" 会被 url.Parse 误认为 scheme=127.0.0.1，因此统一按 http 处理后再解析。
// 参数 raw：原始地址。
// 返回 string：已带 http:// 或 https:// 前缀的地址。
func ensureScheme(raw string) string {
	if len(raw) > 0 && !(len(raw) > 7 && raw[:7] == "http://") && !(len(raw) > 8 && raw[:8] == "https://") {
		return "http://" + raw
	}
	return raw
}

// selfListenPorts 从 WAF 配置的监听地址里提取端口集合，作为自环判定的基准。
// 参数 listens：形如 "0.0.0.0:80"、"[::]:443" 的监听地址列表。
// 返回 map[string]struct{}：端口字符串集合；空串、缺端口或端口非数字的项被跳过。
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
