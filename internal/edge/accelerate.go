package edge

// 本文件实现数据面的页面加速：缓存命中直接返回、回源响应压缩、图片 WebP 转换。
//
// 设计取舍：
//  1. 只用内存 LRU（见 internal/cache），不引入磁盘缓存，避免并发写文件把磁盘打满；
//  2. 缓冲有上限：超过单对象上限立即转为流式透传，绝不把大文件/视频读进内存；
//  3. SSE、WebSocket、非 200、带 Set-Cookie/private 的响应一律绕过缓存；
//  4. 缓存存原始字节，压缩按每个客户端的 Accept-Encoding 现算，避免变体爆炸。

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gowaf/internal/cache"
)

// accelWriter 是带缓存/压缩/WebP 能力的响应包装器。
type accelWriter struct {
	dest     http.ResponseWriter // 下游（通常是 statusRecorder）
	req      *http.Request
	manager  *cache.Manager
	settings cache.Settings
	rule     *cache.Rule
	key      string

	status      int
	wroteHeader bool
	closed      bool
	passthrough bool
	limit       int

	buf bytes.Buffer

	wantCache    bool
	wantCompress bool
	wantWebP     bool
}

// newAccelWriter 创建响应包装器。
// 参数 dest：下游 writer；req：客户端请求；manager：缓存管理器；settings：加速设置；
// rule/key：命中的缓存规则与键，未命中缓存规则时为 nil/空串。
func newAccelWriter(dest http.ResponseWriter, req *http.Request, manager *cache.Manager,
	settings cache.Settings, rule *cache.Rule, key string) *accelWriter {
	w := &accelWriter{
		dest: dest, req: req, manager: manager, settings: settings,
		rule: rule, key: key, status: http.StatusOK, limit: int(settings.MaxObjectBytes),
	}
	if w.limit <= 0 {
		w.limit = cache.DefaultMaxObjectBytes
	}
	// 完全没开任何加速能力时直接透传，不产生额外内存拷贝
	if rule == nil && !settings.Compress && !settings.WebP {
		w.passthrough = true
	}
	return w
}

// Header 返回下游响应头，供 ReverseProxy 写入。
func (w *accelWriter) Header() http.Header { return w.dest.Header() }

// WriteHeader 记录状态码并决定是否缓冲。
func (w *accelWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
	if w.passthrough {
		w.dest.WriteHeader(code)
		return
	}
	h := w.dest.Header()
	// 上游已经压缩过的响应不再二次处理；非 200、非 GET/HEAD 一律透传
	if code != http.StatusOK ||
		(w.req.Method != http.MethodGet && w.req.Method != http.MethodHead) ||
		(h.Get("Content-Encoding") != "" && !strings.EqualFold(h.Get("Content-Encoding"), "identity")) {
		w.flushHeaderOnly()
		return
	}
	ct := h.Get("Content-Type")
	// SSE 必须立即透传，否则事件会被缓冲区卡住
	if strings.Contains(strings.ToLower(ct), "text/event-stream") {
		w.flushHeaderOnly()
		return
	}
	if h.Get("Content-Length") != "" {
		if n, err := strconv.Atoi(h.Get("Content-Length")); err == nil && n > w.limit {
			w.flushHeaderOnly()
			return
		}
	}

	w.wantWebP = w.settings.WebP && cache.WebPConvertible(ct) &&
		acceptsMime(w.req.Header.Get("Accept"), "image/webp")
	w.wantCompress = (w.settings.Compress || w.settings.Brotli) && cache.Compressible(ct)
	w.wantCache = w.rule != nil && cacheableResponse(h)
	if !w.wantCache && !w.wantCompress && !w.wantWebP {
		w.flushHeaderOnly()
	}
}

// Write 写入响应体；超过缓冲上限时自动降级为流式透传。
func (w *accelWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.passthrough {
		return w.dest.Write(p)
	}
	if w.buf.Len()+len(p) > w.limit {
		// 大响应：先把已缓冲内容刷出去，之后全部透传，绝不继续占内存
		if err := w.flushBuffered(); err != nil {
			return 0, err
		}
		return w.dest.Write(p)
	}
	return w.buf.Write(p)
}

// Flush 支持 SSE/流式输出：有缓冲时先刷出再调用下游 Flush。
func (w *accelWriter) Flush() {
	if !w.passthrough {
		if err := w.flushBuffered(); err != nil {
			return
		}
	}
	if f, ok := w.dest.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack 支持 WebSocket 升级：直接透传，不参与缓存/压缩。
func (w *accelWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.passthrough = true
	if hj, ok := w.dest.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("底层不支持 Hijacker")
}

// Close 结束响应：按需执行 WebP、压缩、缓存写入，然后输出。
func (w *accelWriter) Close() {
	if w.closed {
		return
	}
	w.closed = true
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.passthrough {
		return
	}

	h := w.dest.Header()
	body := w.buf.Bytes()
	if body == nil {
		body = []byte{}
	}

	// 1) 缓存写入必须在压缩/WebP 之前：缓存保存原始字节与原始 Content-Type，
	//    后续每个客户端按自己的能力重新协商压缩/图片格式。
	if w.wantCache && len(body) > 0 && w.status == http.StatusOK {
		w.manager.Put(w.key, &cache.Entry{
			Status: w.status,
			Header: cacheHeaders(h),
			Body:   append([]byte(nil), body...),
		}, w.rule.TTL)
	}

	// 2) WebP：仅转换 JPEG/PNG，转换失败保留原图
	if w.wantWebP {
		if webp, ok := cache.ConvertWebP(body); ok {
			body = webp
			h.Set("Content-Type", "image/webp")
			h.Del("Content-Length")
			appendVary(h, "Accept")
		}
	}

	// 3) 压缩：brotli 优先，gzip 兜底；小响应不压缩
	if w.wantCompress && len(body) >= w.settings.CompressMinBytes {
		if data, enc, ok := cache.Compress(body, w.req.Header.Get("Accept-Encoding"), w.settings.Brotli); ok {
			body = data
			h.Set("Content-Encoding", enc)
			h.Del("Content-Length")
			appendVary(h, "Accept-Encoding")
		}
	}
	if h.Get("Content-Length") == "" && len(body) > 0 {
		h.Set("Content-Length", strconv.Itoa(len(body)))
	}
	w.dest.WriteHeader(w.status)
	_, _ = w.dest.Write(body)
}

// flushHeaderOnly 只写响应头，不缓冲响应体。
func (w *accelWriter) flushHeaderOnly() {
	w.passthrough = true
	w.dest.WriteHeader(w.status)
}

// flushBuffered 把当前缓冲内容写出并切换为透传模式。
func (w *accelWriter) flushBuffered() error {
	if w.passthrough {
		return nil
	}
	w.passthrough = true
	w.dest.WriteHeader(w.status)
	if w.buf.Len() > 0 {
		_, err := w.dest.Write(w.buf.Bytes())
		w.buf.Reset()
		return err
	}
	return nil
}

// cacheableResponse 判断响应头是否允许共享缓存。
func cacheableResponse(h http.Header) bool {
	if h.Get("Set-Cookie") != "" {
		return false
	}
	cc := strings.ToLower(h.Get("Cache-Control"))
	if strings.Contains(cc, "no-store") || strings.Contains(cc, "private") || strings.Contains(cc, "no-cache") {
		return false
	}
	if strings.EqualFold(h.Get("X-Accel-Expires"), "0") {
		return false
	}
	return true
}

// cacheHeaders 复制响应头，剔除与缓存/压缩相关、不能跨请求复用的字段。
func cacheHeaders(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vs := range h {
		lk := strings.ToLower(k)
		switch lk {
		case "set-cookie", "content-length", "content-encoding", "transfer-encoding",
			"connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailer", "upgrade", "x-cache", "age":
			continue
		}
		for _, v := range vs {
			out.Add(k, v)
		}
	}
	return out
}

// appendVary 追加 Vary 字段，避免重复。
func appendVary(h http.Header, field string) {
	for _, v := range h.Values("Vary") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), field) {
				return
			}
		}
	}
	h.Add("Vary", field)
}

// acceptsMime 判断 Accept 是否接受指定 MIME 类型。
// 参数 header：Accept 头；参数 want：目标 MIME（如 image/webp）。
func acceptsMime(header, want string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		mt := part
		q := 1.0
		if i := strings.IndexByte(part, ';'); i >= 0 {
			mt = strings.TrimSpace(part[:i])
			for _, attr := range strings.Split(part[i+1:], ";") {
				attr = strings.TrimSpace(attr)
				if strings.HasPrefix(attr, "q=") {
					if v, err := strconv.ParseFloat(strings.TrimSpace(attr[2:]), 64); err == nil {
						q = v
					}
				}
			}
		}
		if q <= 0 {
			continue
		}
		if strings.EqualFold(mt, want) || mt == "*/*" {
			return true
		}
	}
	return false
}

// serveCacheEntry 把缓存对象写回客户端，并执行压缩/WebP 协商。
// 参数 w：响应写出器；r：客户端请求；e：缓存对象；settings：加速设置。
// 返回 int：实际写出的状态码。
func serveCacheEntry(w http.ResponseWriter, r *http.Request, e *cache.Entry, settings cache.Settings) int {
	h := w.Header()
	body := e.Body
	ct := ""
	for k, vs := range e.Header {
		for _, v := range vs {
			h.Add(k, v)
		}
	}
	ct = h.Get("Content-Type")
	if settings.WebP && cache.WebPConvertible(ct) && acceptsMime(r.Header.Get("Accept"), "image/webp") {
		if webp, ok := cache.ConvertWebP(body); ok {
			body = webp
			h.Set("Content-Type", "image/webp")
			appendVary(h, "Accept")
		}
	}
	if (settings.Compress || settings.Brotli) && cache.Compressible(ct) && len(body) >= settings.CompressMinBytes {
		if data, enc, ok := cache.Compress(body, r.Header.Get("Accept-Encoding"), settings.Brotli); ok {
			body = data
			h.Set("Content-Encoding", enc)
			appendVary(h, "Accept-Encoding")
		}
	}
	h.Set("X-Cache", "HIT")
	if e.TTL > 0 {
		age := int(time.Since(e.StoredAt) / time.Second)
		if age < 0 {
			age = 0
		}
		h.Set("Age", strconv.Itoa(age))
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(e.Status)
	_, _ = w.Write(body)
	return e.Status
}
