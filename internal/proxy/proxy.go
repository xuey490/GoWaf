// Package proxy 负责站点路由与反向代理数据面。
//
// 文件分工：
//   - router.go：按 Host 把请求匹配到站点路由（精确 / 泛域名 / 默认站点兜底）；
//   - proxy.go：维护按上游地址缓存的 ReverseProxy，负责转发、XFF 还原与 502 兜底页；
//   - tls.go：多证书管理与 SNI 选择，支持证书目录热加载（续期后无需重启）。
//
// 关键设计：本包不做任何 WAF 检测，只负责"把请求送到正确的上游"。
// 检测在 detect 包，两者通过 app 层串联，保持数据面依赖单一。
package proxy

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"
)

// Upstreams 缓存每个上游地址对应的 ReverseProxy 与连接池。
//
// 为什么按上游缓存而不是每次新建：ReverseProxy 内部持有 Transport，
// 每次新建等于每次重建连接池，keep-alive 完全失效，延迟会翻倍。
type Upstreams struct {
	mu sync.RWMutex                      // 保护 m，转发高并发读、路由重建时写
	m  map[string]*httputil.ReverseProxy // key: 上游地址原文（Route.UpstreamRaw）
}

// NewUpstreams 创建空的代理缓存。
// 无参数；返回 *Upstreams：供 edge 层在转发时调用 Get 取代理实例。
func NewUpstreams() *Upstreams {
	return &Upstreams{m: make(map[string]*httputil.ReverseProxy)}
}

// Get 取得（或创建）某上游地址对应的代理实例。
// 参数 route：命中的站点路由，用其 UpstreamRaw 作缓存键、Upstream 作转发目标。
// 返回 *httputil.ReverseProxy：同一上游全局共享一份（连接池复用），可直接 ServeHTTP。
func (u *Upstreams) Get(route *Route) *httputil.ReverseProxy {
	key := route.UpstreamRaw
	// 先走读锁快速路径：绝大多数请求都是缓存命中
	u.mu.RLock()
	p, ok := u.m[key]
	u.mu.RUnlock()
	if ok {
		return p
	}

	// 未命中才加写锁；拿到写锁后再查一次（双重检查），
	// RUnlock 与 Lock 之间可能已有其他协程把实例建好了
	u.mu.Lock()
	defer u.mu.Unlock()
	if p, ok := u.m[key]; ok {
		return p
	}

	target := route.Upstream
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // 建连超时：上游挂了尽快失败
			KeepAlive: 30 * time.Second, // TCP keepalive 探测间隔
		}).DialContext,
		MaxIdleConns:          512, // 全局空闲连接上限
		MaxIdleConnsPerHost:   256, // 单上游空闲连接上限，扛站点突发流量
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     false, // 后端统一走 HTTP/1.1，避免 h2 与 upgrade 冲突
	}

	rp := &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: -1, // 立即刷出：SSE / 流式响应必须
		// Rewrite 在转发前改写出站请求
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host
			// 追加 XFF，保留原始 Host 供 Apache vhost 匹配
			pr.SetXForwarded()
			// SetXForwarded 会把 XFF 覆盖成「直连对端」地址。WAF 前面还可能站着
			// Cloudflare / 自建 CDN，那样写进去的就是 CDN 节点 IP，Apache 日志、
			// 宝塔的 IP 封禁、站点里的 REMOTE_ADDR 全部失真。
			// edge 层已把解析出的真实客户端 IP 放在 X-Real-IP，用它还原 XFF。
			if realIP := pr.In.Header.Get("X-Real-IP"); realIP != "" {
				pr.Out.Header.Set("X-Forwarded-For", realIP)
			}
			if proto := pr.In.Header.Get("X-Forwarded-Proto"); proto != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", proto)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// 抹掉暴露后端技术栈的响应头，降低被探测的风险
			resp.Header.Del("Server")
			resp.Header.Del("X-Powered-By")
			return nil
		},
		// ErrorHandler 兜底转发失败：客户端断开静默，其余返回统一 502 页
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if isClientGone(err) {
				return
			}
			log.Printf("[proxy] 上游不可达 %s: %v", target.String(), err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(badGatewayPage))
		},
	}

	u.m[key] = rp
	return rp
}

// isClientGone 判断转发错误是否为客户端主动断开——这不该记成上游错误
// 或返回 502（SSE 长连接里客户端随时会走，属正常现象）。
// 参数 err：转发过程中的错误。
// 返回 bool：context canceled 或 broken pipe 视为客户端断开。
func isClientGone(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "broken pipe")
}

// badGatewayPage 上游不可用时返回给客户端的 502 页面，统一文案且不泄露后端报错细节。
const badGatewayPage = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>502 服务暂时不可用</title>
<style>body{font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#334155;
display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;text-align:center}
h1{font-size:22px;margin:0 0 8px}p{color:#64748B;margin:0}</style></head>
<body><div class="box"><h1>502 服务暂时不可用</h1>
<p>上游服务无响应，请稍后重试。</p></div></body></html>`
