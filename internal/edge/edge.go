// Package edge 是数据面入口：一个 http.Handler 走完「识别 → 名单 → 限流 → 检测 → 转发」。
//
// 设计纪律：
//  1. 任何一步出错都不能把请求吞掉（fail-open 优先于 502）；
//  2. 检测只读快照，不查库、不加全局锁；
//  3. 静态资源、白名单路径、健康检查不跑规则引擎；
//  4. IP 白名单是最高优先级：命中后连黑名单、封禁、情报都不看。
package edge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"gowaf/internal/detect"
	"gowaf/internal/guard"
	"gowaf/internal/metrics"
	"gowaf/internal/notify"
	"gowaf/internal/proxy"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// Handler 数据面主处理器：按「识别 → 名单 → 限流 → 检测 → 转发」的顺序处理每个请求。
type Handler struct {
	rt         *runtime.Runtime // 运行时共享上下文：配置、路由、检测引擎、封禁表、事件队列等
	trusted    []netip.Prefix   // 可信代理网段：直连方命中这些网段时才采信 XFF 等转发头
	maxBody    int              // 全局默认请求体检测上限（字节），站点未单独配置时使用
	sampleRate float64          // 正常请求的事件采样率（0~1）；拦截/挑战类事件始终全量记录
	rand       *rand.Rand       // 处理器私有随机源：按采样率抽签用，避免争用全局随机源
}

// New 创建数据面处理器。
// 参数 rt：运行时共享上下文，提供配置、路由表、检测引擎、封禁表等依赖。
// 返回 *Handler：初始化后的数据面处理器；可信代理网段、body 上限、采样率均取自配置，
// 配置缺省或非法时回退内置默认值（body 上限 256KB）。
func New(rt *runtime.Runtime) *Handler {
	var trusted []netip.Prefix
	for _, cidr := range rt.Cfg.Server.TrustedProxies {
		if p, err := netip.ParsePrefix(cidr); err == nil {
			trusted = append(trusted, p)
		}
	}
	maxBody := rt.Cfg.Engine.MaxBodyInspect
	if maxBody <= 0 {
		maxBody = 256 * 1024
	}
	return &Handler{
		rt:         rt,
		trusted:    trusted,
		maxBody:    maxBody,
		sampleRate: rt.Cfg.Storage.SampleRate,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ServeHTTP 实现 http.Handler，是所有客户端流量的必经入口。
// 参数 w：响应写出器；参数 r：客户端请求。
// 返回值无：请求按「白名单 → 黑名单 → 封禁 → 情报 → 限流 → 检测」的顺序过闸，
// 最终要么转发上游（forward），要么被拦截（reject）或下发挑战（serveChallenge）。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// 优先复用上游链路带来的请求 ID，没有则现场生成，并写回响应头便于问题排查
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = newRequestID()
	}
	w.Header().Set("X-Request-ID", reqID)

	clientIP := h.clientIP(r)
	// 按请求头 Host 匹配站点路由；未配置的域名不属于任何站点，直接 404
	route := h.rt.Router().Match(r.Host)
	if route == nil {
		http.Error(w, "404 站点未配置", http.StatusNotFound)
		return
	}

	// 1) 白名单：最高优先级，命中后整条拦截链路直接跳过。
	//
	// 顺序很关键：以前白名单排在黑名单与内存封禁之后，于是「先被封禁、后加白名单」
	// 的 IP 依然会 403（封禁表里还留着那条记录），运维只能手动去封禁管理里解封，
	// 非常反直觉。白名单的语义就是「自己人、无条件放行」，所以放到最前面：
	// 黑名单、封禁、情报、限流、规则引擎一概不参与，也不会有任何自动封禁。
	// 观察模式下同样放行（观察模式只是不拦，不改变放行结果）。
	if h.rt.IPList().IsWhitelisted(clientIP, route.SiteName) {
		h.forward(w, r, route, clientIP, reqID, start, nil)
		return
	}

	// 观察模式：整条拦截链路退化为"只看不拦"。
	//
	// 这是"只记录不拦截"的字面语义。之前这里只放宽了规则判定，而黑名单、
	// 封禁、情报仍然照拦——用户切到观察模式后站点还是一片 403，非常反直觉
	// （线上真实踩过：一次误报触发自动封禁，切成观察模式后依然全站 403）。
	// 现在观察模式下这三种"前置拦截"全部跳过，请求继续走到检测环节，
	// 命中规则照样落事件，只是不处置。
	observe := h.rt.Engine.Mode() == "observe"

	// 2) 黑名单：直接拒绝，不读 body（全局名单 + 本站点名单）
	if !observe && h.rt.IPList().IsBlacklisted(clientIP, route.SiteName) {
		h.reject(w, r, route, clientIP, reqID, start, "block", "ip_blacklist", 403, nil)
		return
	}

	// 3) 内存封禁（全局封禁 + 本站点封禁）
	if !observe && h.rt.Bans.Banned(clientIP.String(), route.SiteName, store.NowMS()) {
		h.reject(w, r, route, clientIP, reqID, start, "block", "ip_banned", 403, nil)
		return
	}

	// 4) 威胁情报：已知恶意 IP 直接拒绝，不进规则引擎。
	// 白名单在最前面已放行，所以「自己人」不会被情报误伤。
	if hit := h.rt.Intel.Match(clientIP, route.SiteName); !observe && hit != "" {
		// 命中即写封禁（可关）：这样封禁管理里能统一看到、能手动解封，
		// 而不是只在事件列表里留一条"情报拦截"。
		banID := h.banFromIntel(clientIP, route.SiteName, hit)
		h.rejectWithBody(w, r, route, clientIP, reqID, start, "block", "intel",
			http.StatusForbidden, "来源 IP 命中威胁情报（"+hit+"）")
		if banID > 0 {
			h.notifyBan(notify.Event{
				IP: clientIP.String(), Site: route.SiteName, Country: h.countryOf(clientIP),
				Source: "intel", Reason: "来源 IP 命中威胁情报（" + hit + "）",
				Method: r.Method, Path: r.URL.Path, At: start,
			})
		}
		return
	}

	// 5) 静态资源/站点白名单路径：跳过检测（性能关键）
	// 注意：只按扩展名跳过会漏掉 /env.js、/runtime-config.js、/aws-exports.js 这类
	// 伪装成静态资源的配置泄露探测，所以敏感文件名必须例外。
	skipDetect := (detect.IsStaticPath(r.URL.Path) && !detect.IsSensitiveAssetPath(r.URL.Path)) ||
		route.IsPathWhitelisted(r.URL.Path)

	// 5.5) 换取通行证的请求：必须放在限流之前。
	// 校验通过后浏览器会先请求这个路径拿 Cookie；如果它自己被 429 拦掉，
	// 用户就会卡在「校验中 → 429 → 刷新 → 校验中」的死循环里。
	if r.URL.Path == h.rt.Chall.PassPath() {
		h.serveChallenge(w, r, clientIP)
		return
	}

	// 5.6) 人机校验：对「可疑但不确定」的流量降级成一次自证，而不是直接拒绝。
	// 触发条件见 challengeReason：跨站来路的页面导航、单 IP 请求频率超过软阈值。
	// 已持有通行证的请求直接放过，不再重复校验（30 分钟有效，绑定 IP）。
	if !skipDetect && !observe {
		if why := h.challengeReason(r, route, clientIP); why != "" && !h.challengePassed(w, r, clientIP) {
			h.serveChallenge(w, r, clientIP)
			// 挑战事件必须落库：看板的「人机校验」计数直接统计 action=challenge 的事件，
			// 之前这里是 sample=false，只进指标不入库，所以那个数字永远是 0。
			h.recordEvent(route, clientIP, reqID, start, r, http.StatusForbidden, "challenge", nil, 0, "", true)
			return
		}
	}

	// 6) 限流：启用站点自定义防护的站点用独立计数（不占全局指标），其余走全局
	if !skipDetect {
		if ok, reason := h.rt.LimiterFor(route.SiteID).Allow(clientIP.String(), store.NowMS()); !ok {
			metrics.BlockedTotal.WithLabelValues(route.SiteName, "rate_limit").Inc()
			reasonText := "请求过于频繁"
			if reason == guard.ReasonMinute {
				reasonText = "每分钟请求数超限"
			}
			h.rejectWithBody(w, r, route, clientIP, reqID, start, "block", string(reason), http.StatusTooManyRequests, reasonText)
			return
		}
	}

	// 7) 读取 body（有上限），供检测与后续转发复用。
	// 返回 skipBody=true 表示 body 超过检测上限或为 chunked，不做 body 规则检测、
	// 原样透传——超大上传（如 64MB 视频）拒绝掉会直接打死业务，体积上限交给上游
	// （Apache LimitRequestBody）控制；URI/Header 检测不受影响。
	body, skipBody, tooLarge := h.readBody(w, r, route)
	if tooLarge {
		h.rejectWithBody(w, r, route, clientIP, reqID, start, "block", "body_too_large",
			http.StatusRequestEntityTooLarge, "请求体超过站点限制")
		return
	}

	// 8) 规则检测
	var result detect.Result
	if !skipDetect && !skipBody && route.DetectBody {
		in := buildInput(r, body)
		in.Site = route.SiteName
		result = h.rt.Engine.Inspect(&in)

		// CRS 第二引擎：覆盖通用攻击特征。两个引擎分数相加后再判动作，
		// 这样「自研规则 30 分 + CRS 一条 error 45 分」也能累计到拦截阈值。
		if crs := h.rt.CRS(); crs != nil && result.Action != "ban" {
			cres := crs.Inspect(&in)
			// 例外对第二引擎同样生效，否则「标记误报」之后照样被拦。
			h.rt.Engine.FilterByException(&cres, route.SiteName, in.Path, in.Params)
			mergeResult(&result, &cres)
		}

		// 站点自定义防护：阈值/模式与全局不同，用站点生效值重判动作。
		// action=ban 是规则的显式动作（命中即封），不参与分数重判。
		if result.Action != "ban" {
			if route.Guard != nil {
				result.Action = decideByGuard(result.Score, route.Guard, h.rt.Engine.Mode())
			} else {
				block, challenge := h.rt.Engine.Scores()
				result.Action = detect.Decide(result.Score, block, challenge, h.rt.Engine.Mode())
			}
		}
		metrics.DetectDuration.Observe(float64(result.ElapsedUS) / 1e6)
		for _, hit := range result.Hits {
			metrics.RuleHits.WithLabelValues(hit.RuleID, hit.Category).Inc()
		}
	}

	switch result.Action {
	case "ban":
		// 高危规则一次即封：不进频次统计，命中这条本身就是证据。
		h.recordDirectBan(clientIP.String(), &result, route.SiteName, r.Method, r.URL.Path)
		h.reject(w, r, route, clientIP, reqID, start, "ban", "rule", http.StatusForbidden, &result)
		return
	case "block":
		h.recordAutoBan(clientIP.String(), route.SiteName, r.Method, r.URL.Path)
		h.reject(w, r, route, clientIP, reqID, start, "block", "rule", http.StatusForbidden, &result)
		return
	case "challenge":
		// 已持有有效通行证则放行；否则下发挑战页，本次请求不转发
		if !h.challengePassed(w, r, clientIP) {
			h.serveChallenge(w, r, clientIP)
			// sample=true：挑战事件必须落库，否则看板「人机校验」永远统计不到
			h.recordEvent(route, clientIP, reqID, start, r, 403, "challenge", &result, 0, "", true)
			return
		}
	}

	// 9) 转发
	h.forward(w, r, route, clientIP, reqID, start, &result)
}

// maxFieldLog 事件字段（路径、Query 等）落库前的最大保留字节数，防止超长 URL 撑爆事件表。
const maxFieldLog = 512

// decideByGuard 用站点级生效阈值重判动作，语义与 detect.Engine.decide 一致。
// 参数 score：两引擎合并后的总分数；参数 g：站点自定义防护配置；参数 globalMode：
// 引擎当前全局模式，仅当站点未显式覆盖模式（g.Mode 为空）时使用。
// 返回 string：处置动作 "pass"/"log"/"challenge"/"block"；观察模式下一律降级为 "log"。
func decideByGuard(score int, g *proxy.SiteGuard, globalMode string) string {
	if score <= 0 {
		return "pass"
	}
	mode := g.Mode
	if mode == "" {
		mode = globalMode
	}
	verdict := "pass"
	switch {
	case score >= g.BlockScore:
		verdict = "block"
	case score >= g.ChallengeScore:
		verdict = "challenge"
	default:
		verdict = "log"
	}
	if mode == "observe" {
		return "log"
	}
	return verdict
}

// buildInput 把 HTTP 请求拼装成规则引擎的检测输入。
// 参数 r：客户端请求；参数 body：已读出的请求体内容（可为空）。
// 返回 detect.Input：包含方法、路径、Query、请求体、UA、请求头、Cookie 等字段；
// Cookie 和 Authorization 头不进 headers（又长又没检测价值），头与 Cookie 各截断到 4KB。
func buildInput(r *http.Request, body []byte) detect.Input {
	headers := ""
	for k, vs := range r.Header {
		lk := strings.ToLower(k)
		// 这些头又长又没信息量，跳过
		if lk == "cookie" || lk == "authorization" {
			continue
		}
		headers += k + ": " + strings.Join(vs, ",") + " "
	}
	if len(headers) > 4096 {
		headers = headers[:4096]
	}
	cookies := ""
	if c := r.Header.Get("Cookie"); c != "" {
		cookies = c
	}
	if len(cookies) > 4096 {
		cookies = cookies[:4096]
	}
	return detect.Input{
		Method:  r.Method,
		Host:    r.Host,
		Path:    r.URL.Path,
		Query:   r.URL.RawQuery,
		Body:    string(body),
		UA:      r.UserAgent(),
		Referer: r.Referer(),
		Headers: headers,
		Cookies: cookies,
		Params:  collectParams(r, body),
	}
}

// mergeResult 把 CRS 第二引擎的结果并入自研引擎结果。
// 参数 dst：自研引擎结果，作为合并目标就地修改；参数 src：CRS 结果，可为 nil 或空结果。
// 返回值无：分数累加、命中与规则 ID 去重合并、分类去重、耗时累加，全部写回 dst。
func mergeResult(dst, src *detect.Result) {
	if src == nil || (len(src.Hits) == 0 && src.Score == 0) {
		return
	}
	dst.Score += src.Score
	dst.Hits = append(dst.Hits, src.Hits...)
	seenRule := make(map[string]struct{}, len(dst.RuleIDs))
	for _, id := range dst.RuleIDs {
		seenRule[id] = struct{}{}
	}
	for _, id := range src.RuleIDs {
		if _, ok := seenRule[id]; ok {
			continue
		}
		seenRule[id] = struct{}{}
		dst.RuleIDs = append(dst.RuleIDs, id)
	}
	for _, c := range src.Categories {
		found := false
		for _, existing := range dst.Categories {
			if existing == c {
				found = true
				break
			}
		}
		if !found {
			dst.Categories = append(dst.Categories, c)
		}
	}
	dst.ElapsedUS += src.ElapsedUS
}

// collectParams 收集 query 与 form 表单的参数名，供规则例外按参数匹配。
// 参数 r：客户端请求；参数 body：请求体内容（仅 x-www-form-urlencoded 会被解析）。
// 返回 []string：去重后的参数名列表，最多 64 个，空名与重复名直接丢弃。
func collectParams(r *http.Request, body []byte) []string {
	limit := 64
	out := make([]string, 0, 16)
	seen := make(map[string]struct{}, 16)
	add := func(k string) {
		if k == "" || len(out) >= limit {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	if q := r.URL.Query(); len(q) > 0 {
		for k := range q {
			add(k)
		}
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") && len(body) > 0 {
		if v, err := url.ParseQuery(string(body)); err == nil {
			for k := range v {
				add(k)
			}
		}
	}
	return out
}

// readBody 读取并截断请求体，读到的内容回填进 r.Body 供规则检测与转发阶段复用。
//
// 参数 w：响应写出器（当前实现未使用，保留以备扩展）；参数 r：客户端请求；
// 参数 route：站点路由，其 MaxBodyBytes 决定本站点的 body 检测上限。
// 返回 (body, skipBody, tooLarge)：body 为读到的请求体内容；skipBody=true 表示跳过
// body 规则检测并原样透传；tooLarge=true 表示 body 已被消费，只能拒绝并返回 413。
//
// 只对「ContentLength 已知且不超过上限」的请求做 body 检测：
//   - 已知超限：body 一个字节都不读，直接透传（上游才是体积上限的裁决者）；
//   - chunked / 未知长度：不读、不检测、透传（读满才知道超限时 body 已被消费，无法回退）；
//   - ContentLength 谎报（声明小、实际大）：读到 max+1 才发现，body 已消费，只能拒绝。
func (h *Handler) readBody(w http.ResponseWriter, r *http.Request, route *proxy.Route) ([]byte, bool, bool) {
	if r.Body == nil || r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil, false, false
	}
	if r.ContentLength > int64(route.MaxBodyBytes) {
		return nil, true, false
	}
	if r.ContentLength < 0 {
		return nil, true, false
	}
	limit := int64(route.MaxBodyBytes) + 1
	buf, err := io.ReadAll(io.LimitReader(r.Body, limit))
	_ = r.Body.Close()
	if err != nil {
		return nil, false, false
	}
	if int64(len(buf)) > int64(route.MaxBodyBytes) {
		return nil, false, true
	}
	// 把已读内容回填进 r.Body：转发阶段还能再读一遍；同步修正 ContentLength 与实际一致
	r.Body = io.NopCloser(bytes.NewReader(buf))
	r.ContentLength = int64(len(buf))
	return buf, false, false
}

// clientIP 还原客户端真实 IP，只在直连方是可信代理时才采信转发头。
// 参数 r：客户端请求。
// 返回 netip.Addr：客户端 IP；直连方不可信时返回直连地址本身，解析失败返回零值。
func (h *Handler) clientIP(r *http.Request) netip.Addr {
	peerStr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peerStr = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(peerStr)
	if err != nil {
		return netip.Addr{}
	}
	peer = peer.Unmap()

	if !h.isTrusted(peer) {
		return peer
	}

	// Cloudflare 会额外给出单值的 CF-Connecting-IP，比 XFF 更可靠：
	// XFF 可能被 Transform Rule 改写，也可能被客户端塞入伪造前缀。
	// 例外：开了 Pseudo IPv4 时它会变成 240.0.0.0/4 的假地址，此时回退到 XFF。
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(cf)); err == nil {
			addr = addr.Unmap()
			if !addr.Is4() || addr.As4()[0] != 240 {
				return addr
			}
		}
	}

	// X-Forwarded-For 从右往左找第一个不可信地址
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			if addr, err := netip.ParseAddr(strings.TrimSpace(parts[i])); err == nil {
				addr = addr.Unmap()
				if !h.isTrusted(addr) {
					return addr
				}
			}
		}
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(xr)); err == nil {
			return addr.Unmap()
		}
	}
	return peer
}

// isTrusted 判断地址是否落在可信代理网段内。
// 参数 addr：待判断的 IP 地址。
// 返回 bool：命中任一可信网段返回 true。
func (h *Handler) isTrusted(addr netip.Addr) bool {
	for _, p := range h.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// forward 把请求转发到上游并记录事件。
// 参数 w：响应写出器；参数 r：客户端请求；参数 route：匹配到的站点路由；
// 参数 ip：客户端 IP；参数 reqID：请求 ID；参数 start：请求进入时间；
// 参数 result：检测结果，白名单等未检测场景可为 nil。
// 返回值无：上游响应原样写回 w；状态码、延迟、采样决定随事件异步落库。
func (h *Handler) forward(w http.ResponseWriter, r *http.Request, route *proxy.Route,
	ip netip.Addr, reqID string, start time.Time, result *detect.Result) {

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	if route.Upstream != nil {
		// 传给上游的协议与真实 IP。X-Real-IP 是 proxy 层还原 XFF 的依据，
		// 解析失败时不能写成 "invalid IP"，否则上游日志会出现垃圾值。
		if ip.IsValid() {
			r.Header.Set("X-Real-IP", ip.String())
		}
		r.Header.Set("X-Forwarded-Proto", scheme(r))
		h.rt.Upstreams.Get(route).ServeHTTP(rec, r)
	} else {
		http.Error(rec, "站点未配置上游", http.StatusBadGateway)
	}

	latency := time.Since(start)
	action := "pass"
	if result != nil && result.Action == "log" {
		action = "log"
	}
	if rec.status >= 500 {
		metrics.UpstreamErrors.WithLabelValues(route.SiteName).Inc()
	}
	h.recordEvent(route, ip, reqID, start, r, rec.status, action, result, latency.Microseconds(), route.UpstreamRaw, shouldSample(h.sampleRate, action, h.rand))
}

// reject 返回默认文案的拒绝响应并记录事件。
// 参数 w：响应写出器；参数 r：客户端请求；参数 route：站点路由；参数 ip：客户端 IP；
// 参数 reqID：请求 ID；参数 start：请求进入时间；参数 action：处置动作（block/ban 等）；
// 参数 reason：拦截原因（写入指标标签与 X-WAF-Reason 响应头）；参数 status：HTTP 状态码；
// 参数 result：检测结果，可为 nil。
// 返回值无：写出 HTML 拦截页并异步落事件。
func (h *Handler) reject(w http.ResponseWriter, r *http.Request, route *proxy.Route,
	ip netip.Addr, reqID string, start time.Time, action, reason string, status int, result *detect.Result) {
	h.rejectWithBody(w, r, route, ip, reqID, start, action, reason, status, "", result)
}

// rejectWithBody 返回携带自定义提示文案的拒绝响应并记录事件，是 reject 的底层实现。
// 参数 w：响应写出器；参数 r：客户端请求；参数 route：站点路由；参数 ip：客户端 IP；
// 参数 reqID：请求 ID；参数 start：请求进入时间；参数 action：处置动作；
// 参数 reason：拦截原因（指标标签）；参数 status：HTTP 状态码；参数 text：拦截页提示文案，
// 为空时使用默认文案；参数 result：可变参数，取第一个元素作为检测结果（可不传）。
// 返回值无：写出 HTML 拦截页并异步落事件。
func (h *Handler) rejectWithBody(w http.ResponseWriter, r *http.Request, route *proxy.Route,
	ip netip.Addr, reqID string, start time.Time, action, reason string, status int, text string, result ...*detect.Result) {

	metrics.BlockedTotal.WithLabelValues(route.SiteName, reason).Inc()

	if text == "" {
		text = "请求被安全策略拦截"
	}
	// 拦截页带请求 ID，且可能被 CDN 的 Cache Everything 规则缓存，
	// 一旦缓存住就会把 403 发给后面所有访问同一 URL 的正常用户（见 noStore 注释）。
	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-WAF-Action", action)
	w.Header().Set("X-WAF-Reason", reason)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, blockPage(status, text, reqID))

	var rs *detect.Result
	if len(result) > 0 {
		rs = result[0]
	}
	h.recordEvent(route, ip, reqID, start, r, status, action, rs, time.Since(start).Microseconds(), "", true)
}

// recordAutoBan 命中拦截规则后累计频次，达阈值自动封禁。
//
// 策略（三档都可在控制台热改，单位见 guard.* 设置项）：
//
//	ban_window 秒内命中 ban_threshold 次 → 封 ban_ttl 秒
//	解封后再次触发视为一次"连击"，连击数达到 ban_max_strikes → 永久封禁
//	两次触发之间超过 ban_strike_reset 秒没有任何命中，连击数清零
//
// 连击数存库（bans 表 source='auto' 的记录数），所以重启、升级都不会丢。
// 自动封禁一律全局：扫描器/爆破往往同时打多个站点，按站点封容易顾此失彼。
// 参数 ip：客户端 IP 字符串；参数 site：命中所在站点名；参数 method、path：命中请求的
// 方法与路径，仅用于告警展示。
// 返回值无：未启用自动封禁或未达阈值时直接返回；达阈值则写内存封禁表、同步落库并告警。
func (h *Handler) recordAutoBan(ip, site, method, path string) {
	if !h.rt.SettingBool(runtime.KeyAutoBan, h.rt.Cfg.Guard.AutoBan) {
		return
	}
	// 以下阈值全部经 Setting* 读取：控制台改完立即生效，无需重启进程
	cfg := h.rt.Cfg.Guard
	threshold := h.rt.SettingInt(runtime.KeyBanThreshold, cfg.BanThreshold)
	windowSec := h.rt.SettingInt(runtime.KeyBanWindow, int(cfg.BanWindow.Duration()/time.Second))
	ttlSec := h.rt.SettingInt(runtime.KeyBanTTL, int(cfg.BanTTL.Duration()/time.Second))
	maxStrikes := h.rt.SettingInt(runtime.KeyBanMaxStrikes, cfg.BanMaxStrikes)
	resetSec := h.rt.SettingInt(runtime.KeyBanStrikeReset, int(cfg.BanStrikeReset.Duration()/time.Second))

	// 固定窗口内命中计数 +1；reached 表示窗口内命中次数已达封禁阈值
	_, reached := h.rt.Hits.Add(ip, store.NowMS(), int64(windowSec)*1000, threshold)
	if !reached {
		return
	}

	// 本次是第几次（解封后再触发才算新的一次）
	strikes := h.autoBanStrikes(ip, int64(resetSec)*1000) + 1

	policy := banPolicy{
		windowSec:  windowSec,
		threshold:  threshold,
		ttlSec:     ttlSec,
		maxTTLSec:  int(cfg.BanMaxTTL.Duration() / time.Second),
		maxStrikes: maxStrikes,
	}
	expires, reason, permanent := policy.decide(strikes, store.NowMS())

	h.rt.Bans.Ban(ip, "", expires)
	metrics.BannedGauge.Set(float64(h.rt.Bans.Size()))

	// 同步落库：下一次触发要靠这条记录算连击数，异步写会数漏
	ban := model.Ban{
		IP:        ip,
		Reason:    reason,
		Source:    "auto",
		Scope:     "global",
		HitCount:  threshold,
		CreatedAt: store.NowMS(),
		ExpiresAt: expires,
	}
	if err := h.rt.Store.Gorm().Create(&ban).Error; err != nil {
		log.Printf("[edge] 写封禁记录失败: %v", err)
	}
	if permanent {
		log.Printf("[edge] %s 连续 %d 次触发，已永久封禁", ip, strikes)
	}
	h.notifyBan(notify.Event{
		IP: ip, Site: site, Country: h.countryOfString(ip),
		Source: "auto", Reason: reason, Method: method, Path: path, At: time.Now(),
	})
}

// recordDirectBan action=ban 规则专用：命中即封，一次都不给尝试机会。
//
// 与 recordAutoBan（N 次阈值 + 连击升级）不同，这里不做任何频次累计——
// 规则本身被标记为高危（如扫描器 UA、爆破端点），触发这一次就该走。
// 封禁时长沿用 ban_ttl 设置，source 记为 rule_ban，面板上一眼可辨。
// 参数 ip：客户端 IP；参数 result：检测结果（取规则 ID 拼原因文案，可为 nil）；
// 参数 site：站点名；参数 method、path：请求方法与路径，用于告警展示。
// 返回值无：直接写内存封禁表、落库并投递告警。
func (h *Handler) recordDirectBan(ip string, result *detect.Result, site, method, path string) {
	cfg := h.rt.Cfg.Guard
	ttlSec := h.rt.SettingInt(runtime.KeyBanTTL, int(cfg.BanTTL.Duration()/time.Second))
	if ttlSec <= 0 {
		ttlSec = 3600
	}
	expires := store.NowMS() + int64(ttlSec)*1000

	h.rt.Bans.Ban(ip, "", expires)
	metrics.BannedGauge.Set(float64(h.rt.Bans.Size()))

	ids := "unknown"
	if result != nil && len(result.RuleIDs) > 0 {
		ids = strings.Join(result.RuleIDs, ",")
	}
	ban := model.Ban{
		IP:        ip,
		Reason:    fmt.Sprintf("命中高危规则（一次封禁）：%s", ids),
		Source:    "rule_ban",
		Scope:     "global",
		HitCount:  1,
		CreatedAt: store.NowMS(),
		ExpiresAt: expires,
	}
	if err := h.rt.Store.Gorm().Create(&ban).Error; err != nil {
		log.Printf("[edge] 写高危规则封禁记录失败: %v", err)
	}
	log.Printf("[edge] %s 命中 action=ban 规则(%s)，已封禁 %d 秒", ip, ids, ttlSec)

	ev := notify.Event{
		IP: ip, Site: site, Country: h.countryOfString(ip),
		Source: "rule_ban", Reason: ban.Reason, Method: method, Path: path, At: time.Now(),
	}
	if result != nil {
		ev.Score = result.Score
		ev.RuleIDs = result.RuleIDs
	}
	h.notifyBan(ev)
}

// banFromIntel 把情报命中的 IP 写进封禁表，返回封禁记录 ID（未启用或写库失败返回 0）。
//
// 只在「首次命中」写入：调用前已经过 Bans.Banned 判定，能走到这里说明内存里还没有
// 这条封禁，因此不会每个请求都插一行。作用域跟随情报源——站点级情报只封该站点，
// 全局情报封全局，这样封禁管理里的「作用域」列能真实反映拦截范围。
// 参数 ip：客户端 IP；参数 site：当前请求的站点名；参数 hit：情报命中描述，
// 以「站点」开头视为站点级情报（只封该站点），否则按全局情报封全局。
// 返回 uint：新建封禁记录的 ID；未启用情报封禁或写库失败返回 0。
func (h *Handler) banFromIntel(ip netip.Addr, site, hit string) uint {
	if !h.rt.SettingBool(runtime.KeyIntelBanOnHit, true) {
		return 0
	}
	ttl := h.rt.SettingInt(runtime.KeyIntelBanTTL, 3600)
	if ttl <= 0 {
		ttl = 3600
	}
	scope, banSite := "global", ""
	if strings.HasPrefix(hit, "站点") {
		scope, banSite = "site", site
	}
	expires := store.NowMS() + int64(ttl)*1000

	h.rt.Bans.Ban(ip.String(), banSite, expires)
	metrics.BannedGauge.Set(float64(h.rt.Bans.Size()))

	ban := model.Ban{
		IP: ip.String(), Reason: "威胁情报：" + hit, Source: "intel",
		Scope: scope, Site: banSite, HitCount: 1,
		CreatedAt: store.NowMS(), ExpiresAt: expires,
	}
	if err := h.rt.Store.Gorm().Create(&ban).Error; err != nil {
		log.Printf("[edge] 写情报封禁记录失败: %v", err)
		return 0
	}
	return ban.ID
}

// notifyBan 投递封禁告警。去重与聚合都在通知器内部完成，这里只管调用。
// 参数 ev：封禁告警事件（IP、站点、原因、来源、命中的请求信息等）。
// 返回值无：通知器未配置时静默跳过。
func (h *Handler) notifyBan(ev notify.Event) {
	if h.rt.Notify == nil {
		return
	}
	h.rt.Notify.Send(ev)
}

// countryOfString 与 countryOf 同义，供只拿到字符串 IP 的调用点使用。
// 参数 ip：字符串形式的 IP 地址。
// 返回 string：ISO 国家代码；Geo 库未加载或查询失败返回空串。
func (h *Handler) countryOfString(ip string) string {
	if h.rt.Geo == nil {
		return ""
	}
	return h.rt.Geo.LookupString(ip)
}

// autoBanStrikes 统计该 IP 在 resetMS 内被自动封禁过几次（不含本次）。
//
// 只统计「没被手动释放」的记录：手动解封会写 released_at，
// 于是解封后连击数自动归零 —— 相当于给了一次干净的机会。
// resetMS<=0 表示不限时间窗。
// 参数 ip：客户端 IP；参数 resetMS：连击计数时间窗（毫秒）。
// 返回 int：符合条件的自动封禁记录数；查询失败记日志并返回 0。
func (h *Handler) autoBanStrikes(ip string, resetMS int64) int {
	q := h.rt.Store.Gorm().Model(&model.Ban{}).
		Where("ip = ? AND source = ? AND released_at = 0", ip, "auto")
	if resetMS > 0 {
		q = q.Where("created_at >= ?", store.NowMS()-resetMS)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		log.Printf("[edge] 统计连击数失败: %v", err)
		return 0
	}
	return int(n)
}

// recordEvent 累加请求指标，并把事件异步入队落库。
// 参数 route：站点路由；参数 ip：客户端 IP；参数 reqID：请求 ID；参数 start：请求进入时间；
// 参数 r：客户端请求；参数 status：响应状态码；参数 action：处置动作；
// 参数 result：检测结果（可为 nil，用于取分数与命中的规则/分类）；
// 参数 latencyUS：处理耗时（微秒）；参数 upstream：上游地址（拦截时为空）；
// 参数 sample：本次事件是否落库（拦截/挑战全量，正常请求按采样率）。
// 返回值无：指标始终累加；sample=false 时只更新指标不落事件。
func (h *Handler) recordEvent(route *proxy.Route, ip netip.Addr, reqID string, start time.Time,
	r *http.Request, status int, action string, result *detect.Result, latencyUS int64, upstream string, sample bool) {

	metrics.RequestsTotal.WithLabelValues(route.SiteName, action).Inc()
	metrics.RequestDuration.WithLabelValues(route.SiteName).Observe(time.Since(start).Seconds())

	if !sample {
		return
	}

	categories := "[]"
	ruleIDs := "[]"
	score := 0
	if result != nil {
		if b, err := json.Marshal(result.Categories); err == nil {
			categories = string(b)
		}
		if b, err := json.Marshal(result.RuleIDs); err == nil {
			ruleIDs = string(b)
		}
		score = result.Score
	}

	h.rt.Events.Enqueue(model.Event{
		TS:         start.UnixMilli(),
		Site:       route.SiteName,
		ClientIP:   ip.String(),
		Country:    h.countryOf(ip),
		Method:     r.Method,
		Host:       r.Host,
		Path:       truncate(r.URL.Path, maxFieldLog),
		Query:      truncate(r.URL.RawQuery, maxFieldLog),
		Status:     status,
		Action:     action,
		Score:      score,
		Categories: categories,
		RuleIDs:    ruleIDs,
		UA:         truncate(r.UserAgent(), 256),
		Referer:    truncate(r.Referer(), 256),
		ReqID:      reqID,
		LatencyUS:  latencyUS,
		Upstream:   upstream,
	})

	// 拦截/挑战实时推给控制台（正常流量不推，避免刷屏）
	if h.rt.OnEvent != nil && action != "pass" {
		h.rt.OnEvent(model.Event{
			TS:         start.UnixMilli(),
			Site:       route.SiteName,
			ClientIP:   ip.String(),
			Country:    h.countryOf(ip),
			Method:     r.Method,
			Host:       r.Host,
			Path:       truncate(r.URL.Path, maxFieldLog),
			Query:      truncate(r.URL.RawQuery, maxFieldLog),
			Status:     status,
			Action:     action,
			Score:      score,
			Categories: categories,
			RuleIDs:    ruleIDs,
			UA:         truncate(r.UserAgent(), 256),
			ReqID:      reqID,
			LatencyUS:  latencyUS,
		})
	}
}

// countryOf 查询 IP 归属地。查询是纯内存二分，失败返回空串不影响事件落库。
// 参数 ip：客户端 IP。
// 返回 string：ISO 国家代码；Geo 库未加载或查询失败返回空串。
func (h *Handler) countryOf(ip netip.Addr) string {
	if h.rt.Geo == nil {
		return ""
	}
	return h.rt.Geo.Lookup(ip)
}

// shouldSample 决定事件是否落库：拦截/挑战/日志动作全量记录，正常请求按采样率。
// 参数 rate：采样率（0~1）；参数 action：处置动作；参数 rnd：随机源，用于按概率抽签。
// 返回 bool：block/challenge/log 返回 true；其余当 rate<=0 返回 false、rate>=1 返回
// true，否则以 rate 的概率随机决定。
func shouldSample(rate float64, action string, rnd *rand.Rand) bool {
	if action == "block" || action == "challenge" || action == "log" {
		return true
	}
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	return rnd.Float64() < rate
}

// scheme 推断请求协议。
// 参数 r：客户端请求。
// 返回 string：TLS 直连返回 "https"；否则返回 X-Forwarded-Proto 头的值（若存在）；
// 都没有时返回 "http"。
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	return "http"
}

// truncate 按字节数截断字符串。
// 参数 s：原始字符串；参数 n：最大保留字节数。
// 返回 string：长度不超过 n 的前缀；按字节截断，超长的多字节字符可能被截在中间。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// statusRecorder 捕获状态码，同时保留 Flusher/Hijacker 能力（WebSocket 必需）。
type statusRecorder struct {
	http.ResponseWriter
	status  int  // 捕获到的响应状态码，初始默认 200
	written bool // 是否已写过响应；状态码只认第一次 WriteHeader/Write
}

// WriteHeader 记录首次写入的状态码并透传给底层 writer；重复调用仍正常透传。
func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Write 隐式 200：未显式调用 WriteHeader 就写 body 时，状态码按 200 记录。
// 参数 b：待写出的字节。返回 (int, error)：写出的字节数与错误，原样透传底层结果。
func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.status = http.StatusOK
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}

// Flush 转发 Flush 调用，支持 SSE 等流式响应；底层不支持 Flusher 时静默忽略。
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack 转发 Hijack 调用，支持 WebSocket 升级；底层不支持时返回错误。
// 返回 (net.Conn, *bufio.ReadWriter, error)：劫持后的连接与读写器，或错误。
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := s.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("底层不支持 Hijacker")
}
