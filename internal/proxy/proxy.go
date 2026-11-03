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
	mu sync.RWMutex
	m  map[string]*httputil.ReverseProxy
}

// NewUpstreams 创建缓存。
func NewUpstreams() *Upstreams {
	return &Upstreams{m: make(map[string]*httputil.ReverseProxy)}
}

// Get 取得（或创建）某上游的代理实例。
func (u *Upstreams) Get(route *Route) *httputil.ReverseProxy {
	key := route.UpstreamRaw
	u.mu.RLock()
	p, ok := u.m[key]
	u.mu.RUnlock()
	if ok {
		return p
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if p, ok := u.m[key]; ok {
		return p
	}

	target := route.Upstream
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   256,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     false, // 后端统一走 HTTP/1.1，避免 h2 与 upgrade 冲突
	}

	rp := &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: -1, // 立即刷出：SSE / 流式响应必须
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
			// 不泄露后端技术栈
			resp.Header.Del("Server")
			resp.Header.Del("X-Powered-By")
			return nil
		},
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

// isClientGone 客户端主动断开不该记成上游错误（SSE 场景很常见）。
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

const badGatewayPage = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>502 服务暂时不可用</title>
<style>body{font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#334155;
display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;text-align:center}
h1{font-size:22px;margin:0 0 8px}p{color:#64748B;margin:0}</style></head>
<body><div class="box"><h1>502 服务暂时不可用</h1>
<p>上游服务无响应，请稍后重试。</p></div></body></html>`
