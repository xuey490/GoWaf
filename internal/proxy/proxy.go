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
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// maxFailoverAttempts 单次请求最多尝试的上游数量，防止全部线路故障时把延迟放大。
const maxFailoverAttempts = 3

// Upstreams 缓存每个站点上游组合对应的 ReverseProxy 与连接池。
//
// 为什么按上游组合缓存而不是每次新建：ReverseProxy 内部持有 Transport，
// 每次新建等于每次重建连接池，keep-alive 完全失效，延迟会翻倍。
type Upstreams struct {
	mu        sync.RWMutex             // 保护 m，转发高并发读、路由重建时写
	m         map[string]*siteUpstream // key: 站点 ID + 上游地址组合
	transport *http.Transport          // 全局共享连接池：按 host 自动分桶
}

// siteUpstream 是一个站点的转发器与目标集合。
type siteUpstream struct {
	proxy   *httputil.ReverseProxy
	targets []*Target
	site    string // 站点名称，仅用于健康状态展示
}

// TargetStatus 是健康检查结果，供控制面展示。
type TargetStatus struct {
	Site     string `json:"site"`
	Addr     string `json:"addr"`
	Line     string `json:"line"`
	Weight   int    `json:"weight"`
	Priority int    `json:"priority"`
	Healthy  bool   `json:"healthy"`
}

// NewUpstreams 创建空的代理缓存。
// 无参数；返回 *Upstreams：供 edge 层在转发时调用 Get 取代理实例。
func NewUpstreams() *Upstreams {
	return &Upstreams{
		m: make(map[string]*siteUpstream),
		transport: &http.Transport{
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
		},
	}
}

// Get 取得（或创建）某站点对应的代理实例。
// 参数 route：命中的站点路由；多上游时由内部 Transport 做健康目标选择与故障切换。
// 返回 http.Handler：同一上游组合全局共享一份（连接池复用），可直接 ServeHTTP。
func (u *Upstreams) Get(route *Route) http.Handler {
	key := routeUpstreamKey(route)
	// 先走读锁快速路径：绝大多数请求都是缓存命中
	u.mu.RLock()
	s, ok := u.m[key]
	u.mu.RUnlock()
	if ok {
		return s.proxy
	}

	// 未命中才加写锁；拿到写锁后再查一次（双重检查），
	// RUnlock 与 Lock 之间可能已有其他协程把实例建好了
	u.mu.Lock()
	defer u.mu.Unlock()
	if s, ok := u.m[key]; ok {
		return s.proxy
	}

	target := route.Upstream
	targets := route.Targets
	if len(targets) == 0 && target != nil {
		t := &Target{URL: target, Raw: route.UpstreamRaw, Weight: 1, Priority: 1}
		t.Healthy.Store(true)
		targets = []*Target{t}
	}

	rp := &httputil.ReverseProxy{
		Transport:     &failoverTransport{base: u.transport, targets: targets},
		FlushInterval: -1, // 立即刷出：SSE / 流式响应必须
		// Rewrite 在转发前改写出站请求；实际目标由 failoverTransport 按健康度切换
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
			log.Printf("[proxy] 上游不可达 %s: %v", route.UpstreamRaw, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(badGatewayPage))
		},
	}

	u.m[key] = &siteUpstream{proxy: rp, targets: targets, site: route.SiteName}
	return rp
}

// routeUpstreamKey 生成站点上游组合的缓存键：站点 ID + 排序后的地址列表。
// 地址排序保证数据库返回顺序变化时仍能复用连接池。
func routeUpstreamKey(route *Route) string {
	raws := make([]string, 0, len(route.Targets)+1)
	for _, t := range route.Targets {
		raws = append(raws, t.Raw)
	}
	if len(raws) == 0 {
		raws = append(raws, route.UpstreamRaw)
	}
	sort.Strings(raws)
	return fmt.Sprintf("%d|%s", route.SiteID, strings.Join(raws, ","))
}

// failoverTransport 按健康度与优先级选择上游，连接失败自动切换到下一条。
//
// 只在请求体可重放（无 body 或带 GetBody）时重试；大文件/流式上传不换目标，
// 避免读到一半的 body 被重复发送。
type failoverTransport struct {
	base    http.RoundTripper
	targets []*Target
}

// RoundTrip 实现 http.RoundTripper。
func (t *failoverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	order := targetOrder(t.targets)
	if len(order) == 0 {
		return nil, fmt.Errorf("站点未配置上游")
	}
	replayable := req.Body == nil || req.GetBody != nil
	var lastErr error
	for i, target := range order {
		if i >= maxFailoverAttempts {
			break
		}
		out := req.Clone(req.Context())
		// 只切换 scheme/host：多线路共用同一路径前缀，Rewrite 已按主上游完成路径拼接
		out.URL.Scheme = target.URL.Scheme
		out.URL.Host = target.URL.Host
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			if out.Body != nil {
				_ = out.Body.Close()
			}
			out.Body = body
		}
		resp, err := t.base.RoundTrip(out)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !replayable {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("上游均不可用")
	}
	return nil, lastErr
}

// targetOrder 生成尝试顺序：健康目标优先；同优先级内按权重随机打散。
// 全部目标都不健康时退回全部目标（fail-open），避免健康检查误判造成全站 502。
func targetOrder(targets []*Target) []*Target {
	if len(targets) == 0 {
		return nil
	}
	all := append([]*Target(nil), targets...)
	healthy := make([]*Target, 0, len(all))
	for _, t := range all {
		if t.Healthy.Load() {
			healthy = append(healthy, t)
		}
	}
	if len(healthy) == 0 {
		healthy = all
	}
	sort.SliceStable(healthy, func(i, j int) bool {
		return healthy[i].Priority < healthy[j].Priority
	})
	out := make([]*Target, 0, len(healthy))
	for i := 0; i < len(healthy); {
		j := i
		for j < len(healthy) && healthy[j].Priority == healthy[i].Priority {
			j++
		}
		out = append(out, weightedOrder(healthy[i:j])...)
		i = j
	}
	return out
}

// weightedOrder 对同优先级目标做不放回的加权随机排序。
func weightedOrder(group []*Target) []*Target {
	pool := append([]*Target(nil), group...)
	out := make([]*Target, 0, len(pool))
	for len(pool) > 0 {
		total := 0
		for _, t := range pool {
			if t.Weight > 0 {
				total += t.Weight
			} else {
				total++
			}
		}
		if total <= 0 {
			break
		}
		pick := rand.Intn(total)
		idx := 0
		for i, t := range pool {
			w := t.Weight
			if w <= 0 {
				w = 1
			}
			if pick < w {
				idx = i
				break
			}
			pick -= w
		}
		out = append(out, pool[idx])
		pool = append(pool[:idx], pool[idx+1:]...)
	}
	return out
}

// CheckHealth 对所有站点上游做一次健康探测：任何 <500 的 HTTP 响应视为存活。
// 参数 ctx：探测上下文，调用方负责超时控制。
func (u *Upstreams) CheckHealth(ctx context.Context) {
	u.mu.RLock()
	handlers := make([]*siteUpstream, 0, len(u.m))
	for _, s := range u.m {
		handlers = append(handlers, s)
	}
	u.mu.RUnlock()

	client := &http.Client{
		Transport: u.transport,
		Timeout:   3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	seen := make(map[string]struct{}, 16)
	for _, s := range handlers {
		for _, t := range s.targets {
			if _, ok := seen[t.Raw]; ok {
				continue
			}
			seen[t.Raw] = struct{}{}
			ok := probeTarget(ctx, client, t)
			t.Healthy.Store(ok)
		}
	}
}

// probeTarget 探测单个上游地址。
func probeTarget(ctx context.Context, client *http.Client, t *Target) bool {
	if t == nil || t.URL == nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, t.URL.String(), nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}

// RunHealthChecks 周期执行健康检查，直到上下文取消。
// 参数 interval：检查间隔，<=0 时取 10 秒。
func (u *Upstreams) RunHealthChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	u.CheckHealth(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			u.CheckHealth(ctx)
		}
	}
}

// TargetStatuses 返回当前全部目标健康状态，供控制面展示。
func (u *Upstreams) TargetStatuses() []TargetStatus {
	u.mu.RLock()
	defer u.mu.RUnlock()
	seen := make(map[string]struct{}, 8)
	out := make([]TargetStatus, 0, 8)
	for _, s := range u.m {
		for _, t := range s.targets {
			if _, ok := seen[t.Raw]; ok {
				continue
			}
			seen[t.Raw] = struct{}{}
			out = append(out, TargetStatus{
				Site: s.site, Addr: t.Raw, Line: t.Line, Weight: t.Weight,
				Priority: t.Priority, Healthy: t.Healthy.Load(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Addr == out[j].Addr {
			return out[i].Line < out[j].Line
		}
		return out[i].Addr < out[j].Addr
	})
	return out
}

// CheckHealthOnce 对指定地址立即探测一次，供「站点连通性测试」调用。
// 参数 addr：完整上游地址。返回 bool：可连通返回 true。
func (u *Upstreams) CheckHealthOnce(addr string) bool {
	parsed, err := url.Parse(strings.TrimSpace(addr))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	client := &http.Client{
		Transport: u.transport,
		Timeout:   3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return probeTarget(context.Background(), client, &Target{URL: parsed})
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
