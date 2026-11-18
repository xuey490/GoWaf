package edge

// 响应改写层：在「上游 → 加速 writer」之间插入可选的响应体变换阶段。
//
// 当前承载两个能力：
//   1. respfilter 响应敏感信息过滤（observe / mask）；
//   2. dprotect 动态防护（HTML 文本节点加密）。
//
// 管线顺序（关键设计）：
//
//	上游 ──> transformWriter ──> accelWriter ──> statusRecorder ──> 客户端
//
// transformWriter 在 accelWriter 之前，有两个原因：
//   - accelWriter 会在 Close 时按客户端能力重新压缩/转 WebP，变换必须发生在
//     压缩之前，否则拿到的是压缩字节，扫描与 DOM 加密都无从谈起；
//   - 请求侧把发往上游的 Accept-Encoding 置为 identity（见 forward），上游回的
//     就是明文，这里不需要处理解压。
//
// 动态防护开启的站点，页面缓存会整段跳过（forward 的调用方在缓存查找前决定）：
// 每个请求的密钥都是一次性的，缓存一份密文页会让后面所有访客解密失败。
//
// 行为约束沿用 accelWriter 的既有约定：缓冲有上限、超限降级透传、SSE/WS 永远
// 不缓冲、宽限期后才响应 Flush（ReverseProxy 的 FlushInterval=-1 会被缓冲住）。

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"gowaf/internal/detect"
	"gowaf/internal/dprotect"
	"gowaf/internal/metrics"
	"gowaf/internal/proxy"
	"gowaf/internal/respfilter"
	"gowaf/internal/tamper"
)

// responseStage 一个响应体变换阶段。
// Apply 收到的永远是明文响应体；返回值是改写后的响应体与归因结果（可为空）。
type responseStage interface {
	// Transformable 判断这份响应是否属于本阶段的处理范围（状态码 + Content-Type）。
	Transformable(status int, h http.Header) bool
	// Limit 本阶段允许缓冲的最大字节数。
	Limit() int
	// Apply 对缓冲的明文响应体做变换。
	Apply(status int, h http.Header, body []byte) ([]byte, detect.Result)
}

// respFilterStage 敏感信息过滤阶段。
type respFilterStage struct {
	s respfilter.Settings
}

// Transformable 只处理 2xx 文本类响应（判断逻辑在 respfilter 包内）。
func (st respFilterStage) Transformable(status int, h http.Header) bool {
	return respfilter.Transformable(status, h)
}

// Limit 取设置中的扫描上限。
func (st respFilterStage) Limit() int {
	if st.s.MaxBytes <= 0 {
		return respfilter.DefaultMaxBytes
	}
	return st.s.MaxBytes
}

// Apply 执行扫描/脱敏，并按模式累加指标。
func (st respFilterStage) Apply(status int, h http.Header, body []byte) ([]byte, detect.Result) {
	out, res := respfilter.Apply(&st.s, status, h, body)
	for _, hit := range res.Hits {
		metrics.RespFilterHits.WithLabelValues(hit.RuleID, st.s.Mode).Inc()
	}
	return out, res
}

// dProtectStage 动态防护阶段。
type dProtectStage struct {
	m *dprotect.Manager
	s dprotect.Settings
}

// Transformable 只加密 200 的 text/html 响应。
func (st dProtectStage) Transformable(status int, h http.Header) bool {
	if status != http.StatusOK {
		return false
	}
	return strings.HasPrefix(strings.ToLower(h.Get("Content-Type")), "text/html")
}

// Limit 取设置中的页面体积上限。
func (st dProtectStage) Limit() int {
	if st.s.MaxBytes <= 0 {
		return dprotect.DefaultMaxBytes
	}
	return st.s.MaxBytes
}

// Apply 加密 HTML 文本节点；失败按 fail-open 透传原文。
func (st dProtectStage) Apply(_ int, _ http.Header, body []byte) ([]byte, detect.Result) {
	out, err := st.m.Protect(body)
	if err != nil {
		return body, detect.Result{}
	}
	return out, detect.Result{}
}

// tamperStage 防篡改比对阶段：与基线比对完整性，不一致按模式记录或替换。
type tamperStage struct {
	m    *tamper.Manager
	s    tamper.Settings
	site string // 基线按「站点+路径」存储
	path string // 受保护的精确路径（构建 stage 时已由 Applies 过滤）
}

// Transformable 只比对 200 且类型在受保护集合内的响应。
func (st tamperStage) Transformable(status int, h http.Header) bool {
	return tamper.Transformable(status, h.Get("Content-Type"))
}

// Limit 取设置中的基线副本上限。
func (st tamperStage) Limit() int {
	if st.s.MaxBytes <= 0 {
		return tamper.DefaultMaxBytes
	}
	return st.s.MaxBytes
}

// Apply 比对响应体与基线：一致放行；不一致时 observe 记录、block 用基线副本替换。
// 没有基线（管理员尚未生成）时放行——防篡改的语义是「与可信基线比对」，
// 没有可信基线就没有比对依据，绝不把「第一次见到的内容」当成可信内容。
func (st tamperStage) Apply(_ int, _ http.Header, body []byte) ([]byte, detect.Result) {
	res := detect.Result{Hits: []detect.Hit{}, Categories: []string{}, RuleIDs: []string{}}
	b, ok := st.m.BaselineFor(st.site, st.path)
	if !ok || len(body) == 0 {
		return body, res
	}
	if st.m.Matches(b, body) {
		metrics.TamperHits.WithLabelValues(st.site, "match").Inc()
		return body, res
	}
	// 不一致：记录归因（RuleIDs 非空 → 事件必记录，事件页可搜 tamper）
	res.Hits = append(res.Hits, detect.Hit{
		RuleID: "tamper", Name: "防篡改基线不一致：" + st.path,
		Category: "tamper", Score: 0, Target: "response",
	})
	res.RuleIDs = append(res.RuleIDs, "tamper")
	res.Categories = append(res.Categories, "tamper")

	if st.s.Mode == tamper.ModeBlock {
		if copyBody := st.m.Copy(b); copyBody != nil {
			metrics.TamperHits.WithLabelValues(st.site, "replaced").Inc()
			res.Action = "block"
			return copyBody, res
		}
		// 有哈希没副本（超大响应）：只能记录，不能凭空还原
		metrics.TamperHits.WithLabelValues(st.site, "mismatch").Inc()
		return body, res
	}
	metrics.TamperHits.WithLabelValues(st.site, "mismatch").Inc()
	return body, res
}

// transformWriter 响应改写 writer：缓冲明文响应，Close 时依次执行各阶段。
type transformWriter struct {
	dest   http.ResponseWriter // 下游（accelWriter）
	stages []responseStage     // 当前请求适用的变换阶段（构建时已按站点/路径筛过）
	limit  int                 // 各阶段上限的最大值

	status      int
	wroteHeader bool
	closed      bool
	passthrough bool
	buf         bytes.Buffer
	firstWrite  time.Time
	results     []detect.Result // 各阶段产生的归因结果，Close 后由 forward 并入事件
}

// newTransformWriter 创建响应改写 writer。
// 参数 dest：下游 writer（accelWriter）；参数 stages：已按站点/路径筛过的阶段列表（非空）。
// 返回 *transformWriter：缓冲上限取各阶段 Limit 的最大值。
func newTransformWriter(dest http.ResponseWriter, stages []responseStage) *transformWriter {
	w := &transformWriter{dest: dest, stages: stages, status: http.StatusOK}
	for _, st := range stages {
		if st.Limit() > w.limit {
			w.limit = st.Limit()
		}
	}
	if w.limit <= 0 {
		w.limit = respfilter.DefaultMaxBytes
	}
	return w
}

// Header 透传下游响应头。
func (w *transformWriter) Header() http.Header { return w.dest.Header() }

// WriteHeader 记录状态码；响应不属于任何阶段的处理范围时立即转透传。
func (w *transformWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code

	active := false
	for _, st := range w.stages {
		if st.Transformable(code, w.dest.Header()) {
			active = true
			break
		}
	}
	if !active {
		w.passthrough = true
		w.dest.WriteHeader(code)
		return
	}
	// 正文会被改写：上游的 Content-Length 与 ETag 从此不可信。
	// 头表与 accelWriter 共享，accelWriter 会在自己的 Close 里重新设置 Content-Length。
	w.dest.Header().Del("Content-Length")
	w.dest.Header().Del("Content-Encoding")
	w.dest.Header().Del("ETag")
}

// Write 缓冲响应体；超过上限时降级为透传，绝不继续占内存。
func (w *transformWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.passthrough {
		return w.dest.Write(p)
	}
	if w.buf.Len()+len(p) > w.limit {
		if err := w.flushBuffered(); err != nil {
			return 0, err
		}
		return w.dest.Write(p)
	}
	if w.firstWrite.IsZero() {
		w.firstWrite = time.Now()
	}
	return w.buf.Write(p)
}

// Flush 在宽限期内保持缓冲（静态页通常在宽限期内读完，变换才有机会执行），
// 超过宽限期恢复边收边发，保证慢速/流式响应不被变换层拖住。
func (w *transformWriter) Flush() {
	if !w.passthrough {
		if w.firstWrite.IsZero() || time.Since(w.firstWrite) < flushGrace {
			return
		}
		if err := w.flushBuffered(); err != nil {
			return
		}
	}
	if f, ok := w.dest.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack 支持 WebSocket 升级：直接透传，不参与任何变换。
func (w *transformWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.passthrough = true
	if hj, ok := w.dest.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("底层不支持 Hijacker")
}

// Close 结束响应：对缓冲的明文依次执行各阶段变换，再交给下游。
// 单阶段失败（如 HTML 解析失败）只影响该阶段：拿原文继续下一阶段（fail-open）。
func (w *transformWriter) Close() {
	if w.closed {
		return
	}
	w.closed = true
	if w.passthrough {
		return
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	body := w.buf.Bytes()
	for _, st := range w.stages {
		if len(body) == 0 || !st.Transformable(w.status, w.dest.Header()) {
			continue
		}
		out, res := st.Apply(w.status, w.dest.Header(), body)
		if len(res.Hits) > 0 {
			w.results = append(w.results, res)
		}
		body = out
	}
	w.dest.WriteHeader(w.status)
	if len(body) > 0 {
		_, _ = w.dest.Write(body)
	}
}

// flushBuffered 把缓冲内容写出并切换为透传模式。
func (w *transformWriter) flushBuffered() error {
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

// activeStages 按站点与请求筛选适用的响应变换阶段。
// 参数 route：站点路由；参数 r：客户端请求。
// 返回 []responseStage：未启用任何响应侧能力时返回 nil（forward 不包改写层）。
func (h *Handler) activeStages(route *proxy.Route, r *http.Request) []responseStage {
	var stages []responseStage
	if s := h.rt.RespFilter(); respfilter.Applies(s, r.URL.Path) {
		stages = append(stages, respFilterStage{s: *s})
	}
	if s := h.rt.DProtect(); dprotect.Applies(s, r.Method, r.URL.Path) {
		stages = append(stages, dProtectStage{m: h.rt.DProtectMgr(), s: *s})
	}
	if ts := h.rt.TamperSettings(); tamper.Applies(ts, r.Method, r.URL.Path) {
		stages = append(stages, tamperStage{m: h.rt.Tamper, s: *ts, site: route.SiteName, path: r.URL.Path})
	}
	return stages
}

// dprotectChallengeGate 动态防护的人机校验门禁：开启 Challenge 的站点，未通过
// PoW 的访客先完成校验再拿正文。与既有挑战链路共用通行证（30 分钟有效）。
// 返回 bool：true 表示已下发挑战页，调用方应直接结束请求。
func (h *Handler) dprotectChallengeGate(route *proxy.Route, r *http.Request, ip netip.Addr) bool {
	s := h.rt.DProtect()
	if s == nil || !s.Enabled || !s.Challenge {
		return false
	}
	if !dprotect.Applies(s, r.Method, r.URL.Path) {
		return false
	}
	return !h.challengePassed(nil, r, ip)
}
