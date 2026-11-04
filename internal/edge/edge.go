// Package edge 是数据面入口：一个 http.Handler 走完「识别 → 名单 → 限流 → 检测 → 转发」。
//
// 设计纪律：
//  1. 任何一步出错都不能把请求吞掉（fail-open 优先于 502）；
//  2. 检测只读快照，不查库、不加全局锁；
//  3. 静态资源、白名单路径、健康检查不跑规则引擎。
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

// Handler 数据面主处理器。
type Handler struct {
	rt         *runtime.Runtime
	trusted    []netip.Prefix
	maxBody    int
	sampleRate float64
	rand       *rand.Rand
}

// New 创建数据面处理器。
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

// ServeHTTP 实现 http.Handler。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = newRequestID()
	}
	w.Header().Set("X-Request-ID", reqID)

	clientIP := h.clientIP(r)
	route := h.rt.Router().Match(r.Host)
	if route == nil {
		http.Error(w, "404 站点未配置", http.StatusNotFound)
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

	// 1) 黑名单：直接拒绝，不读 body（全局名单 + 本站点名单）
	if !observe && h.rt.IPList().IsBlacklisted(clientIP, route.SiteName) {
		h.reject(w, r, route, clientIP, reqID, start, "block", "ip_blacklist", 403, nil)
		return
	}

	// 2) 内存封禁（全局封禁 + 本站点封禁）
	if !observe && h.rt.Bans.Banned(clientIP.String(), route.SiteName, store.NowMS()) {
		h.reject(w, r, route, clientIP, reqID, start, "block", "ip_banned", 403, nil)
		return
	}

	// 3) 白名单：完全跳过检测与限流
	if h.rt.IPList().IsWhitelisted(clientIP, route.SiteName) {
		h.forward(w, r, route, clientIP, reqID, start, nil)
		return
	}

	// 3.5) 威胁情报：已知恶意 IP 直接拒绝，不进规则引擎。
	// 白名单在上面已放行，所以「自己人」不会被情报误伤。
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

	// 4) 静态资源/站点白名单路径：跳过检测（性能关键）
	// 注意：只按扩展名跳过会漏掉 /env.js、/runtime-config.js、/aws-exports.js 这类
	// 伪装成静态资源的配置泄露探测，所以敏感文件名必须例外。
	skipDetect := (detect.IsStaticPath(r.URL.Path) && !detect.IsSensitiveAssetPath(r.URL.Path)) ||
		route.IsPathWhitelisted(r.URL.Path)

	// 5) 限流：启用站点自定义防护的站点用独立计数（不占全局指标），其余走全局
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

	// 6) 读取 body（有上限），供检测与后续转发复用。
	// 返回 skipBody=true 表示 body 超过检测上限或为 chunked，不做 body 规则检测、
	// 原样透传——超大上传（如 64MB 视频）拒绝掉会直接打死业务，体积上限交给上游
	// （Apache LimitRequestBody）控制；URI/Header 检测不受影响。
	body, skipBody, tooLarge := h.readBody(w, r, route)
	if tooLarge {
		h.rejectWithBody(w, r, route, clientIP, reqID, start, "block", "body_too_large",
			http.StatusRequestEntityTooLarge, "请求体超过站点限制")
		return
	}

	// 7) 规则检测
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
		if !h.challengePassed(w, r, clientIP) {
			h.serveChallenge(w, r, clientIP)
			h.recordEvent(route, clientIP, reqID, start, r, 403, "challenge", &result, 0, "", false)
			return
		}
	}

	// 8) 转发
	h.forward(w, r, route, clientIP, reqID, start, &result)
}

// fieldValue 上限。
const maxFieldLog = 512

// decideByGuard 用站点级生效阈值重判动作，语义与 detect.Engine.decide 一致。
// mode 传引擎当前全局模式，仅当站点未显式覆盖模式时使用。
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

// mergeResult 把 CRS 结果并入自研引擎结果。
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

// readBody 读取并回填请求体。
//
// 返回值：body、skipBody（跳过 body 检测并原样透传）、tooLarge（拒绝 413）。
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
	r.Body = io.NopCloser(bytes.NewReader(buf))
	r.ContentLength = int64(len(buf))
	return buf, false, false
}

// clientIP 只在直连方是可信代理时才采信 XFF。
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

func (h *Handler) isTrusted(addr netip.Addr) bool {
	for _, p := range h.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// forward 转发并记录事件。
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

// reject 返回拒绝响应并记录事件。
func (h *Handler) reject(w http.ResponseWriter, r *http.Request, route *proxy.Route,
	ip netip.Addr, reqID string, start time.Time, action, reason string, status int, result *detect.Result) {
	h.rejectWithBody(w, r, route, ip, reqID, start, action, reason, status, "", result)
}

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
func (h *Handler) recordAutoBan(ip, site, method, path string) {
	if !h.rt.SettingBool(runtime.KeyAutoBan, h.rt.Cfg.Guard.AutoBan) {
		return
	}
	cfg := h.rt.Cfg.Guard
	threshold := h.rt.SettingInt(runtime.KeyBanThreshold, cfg.BanThreshold)
	windowSec := h.rt.SettingInt(runtime.KeyBanWindow, int(cfg.BanWindow.Duration()/time.Second))
	ttlSec := h.rt.SettingInt(runtime.KeyBanTTL, int(cfg.BanTTL.Duration()/time.Second))
	maxStrikes := h.rt.SettingInt(runtime.KeyBanMaxStrikes, cfg.BanMaxStrikes)
	resetSec := h.rt.SettingInt(runtime.KeyBanStrikeReset, int(cfg.BanStrikeReset.Duration()/time.Second))

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
func (h *Handler) notifyBan(ev notify.Event) {
	if h.rt.Notify == nil {
		return
	}
	h.rt.Notify.Send(ev)
}

// countryOfString 与 countryOf 同义，供只拿到字符串 IP 的调用点使用。
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

// recordEvent 异步落事件。
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

// countryOf 查归属地。查询是纯内存二分，失败返回空串不影响事件落库。
func (h *Handler) countryOf(ip netip.Addr) string {
	if h.rt.Geo == nil {
		return ""
	}
	return h.rt.Geo.Lookup(ip)
}

// shouldSample 拦截/挑战全量记录，正常请求按采样率。
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

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	return "http"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// statusRecorder 捕获状态码，同时保留 Flusher/Hijacker 能力（WebSocket 必需）。
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

// WriteHeader 记录状态码。
func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Write 隐式 200。
func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.status = http.StatusOK
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}

// Flush 支持 SSE。
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack 支持 WebSocket 升级。
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := s.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("底层不支持 Hijacker")
}
