// Package bot 实现 Bot 识别与行为管理。
//
// 能力：
//   - UA / Path / IP / rDNS 四类 Bot 策略；
//   - 搜索引擎 Bot 的正向-反向 DNS 双重验证，避免伪造 Googlebot UA；
//   - 蜜罐路径：正常用户不会访问，命中即判定恶意；
//   - 轻量行为评分：请求频率 + 可疑 UA，超过阈值按配置动作处置。
//
// 所有配置在 Reload 时编译成快照；DNS 查询带 TTL 缓存，不会在每个请求上阻塞。
package bot

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Action 是 Bot 命中后的处置动作。
type Action string

const (
	ActionAllow     Action = "allow"
	ActionLog       Action = "log"
	ActionChallenge Action = "challenge"
	ActionBlock     Action = "block"
	ActionBan       Action = "ban"
)

// PolicyDef 是数据库 Bot 策略到引擎的输入。
type PolicyDef struct {
	ID        uint
	Name      string
	Site      string
	Category  string
	Action    Action
	MatchType string
	Pattern   string
	Priority  int
	Enabled   bool
}

// HoneypotDef 是蜜罐路径定义。
type HoneypotDef struct {
	ID      uint
	Site    string
	Path    string
	Enabled bool
}

// Settings 是 Bot 管理的全局开关。
type Settings struct {
	Enabled        bool          // 总开关
	VerifyRDNS     bool          // 是否对搜索引擎 Bot 做 rDNS 验证
	ScoreThreshold int           // 行为评分阈值（窗口内请求次数）
	Action         Action        // 未命中显式策略时的默认动作
	Window         time.Duration // 行为统计窗口
	CacheTTL       time.Duration // rDNS 结果缓存时长
}

// Verdict 是一次 Bot 判定结果。
type Verdict struct {
	Matched  bool   `json:"matched"`
	Action   Action `json:"action"`
	Category string `json:"category"`
	BotName  string `json:"botName"`
	Verified bool   `json:"verified"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
}

// Request 是 Bot 求值上下文。
type Request struct {
	IP    netip.Addr
	UA    string
	Path  string
	Site  string
	Query string
	Host  string
}

// Manager 是 Bot 模块的运行时入口，配置快照可并发读取。
type Manager struct {
	state    atomic.Pointer[state]
	tracker  *tracker
	resolver *resolver
}

// state 是不可变配置快照。
type state struct {
	policies  []*compiledPolicy
	honeypots []compiledHoneypot
	settings  Settings
}

// compiledPolicy 是编译后的 Bot 策略。
type compiledPolicy struct {
	id        uint
	name      string
	site      string
	category  string
	action    Action
	matchType string
	re        *regexp.Regexp
	prefix    *netip.Prefix
	pattern   string
	priority  int
}

// compiledHoneypot 是编译后的蜜罐路径。
type compiledHoneypot struct {
	site  string
	path  string
	exact bool
}

// NewManager 创建 Bot 管理器；默认关闭，等待 Reload 注入配置。
func NewManager() *Manager {
	m := &Manager{
		tracker:  newTracker(10 * time.Minute),
		resolver: newResolver(10 * time.Minute),
	}
	m.state.Store(&state{settings: Settings{Window: time.Minute, CacheTTL: 10 * time.Minute}})
	return m
}

// Reload 编译 Bot 配置并替换快照。
// 参数 policies：Bot 策略；honeypots：蜜罐路径；st：全局开关。
// 返回 error：任一正则/网段非法时整体失败，旧快照保留。
func (m *Manager) Reload(policies []PolicyDef, honeypots []HoneypotDef, st Settings) error {
	if st.Window <= 0 {
		st.Window = time.Minute
	}
	if st.CacheTTL <= 0 {
		st.CacheTTL = 10 * time.Minute
	}
	if st.ScoreThreshold <= 0 {
		st.ScoreThreshold = 30
	}
	if st.Action == "" {
		st.Action = ActionChallenge
	}
	next := &state{settings: st}
	for _, d := range policies {
		if !d.Enabled {
			continue
		}
		p := &compiledPolicy{
			id: d.ID, name: d.Name, site: strings.TrimSpace(d.Site),
			category: strings.TrimSpace(d.Category), action: d.Action,
			matchType: strings.ToLower(strings.TrimSpace(d.MatchType)),
			pattern:   strings.TrimSpace(d.Pattern), priority: d.Priority,
		}
		if p.category == "" {
			p.category = "unknown"
		}
		if p.action == "" {
			p.action = ActionChallenge
		}
		switch p.action {
		case ActionAllow, ActionLog, ActionChallenge, ActionBlock, ActionBan:
		default:
			return &policyError{name: d.Name, msg: "动作非法: " + string(p.action)}
		}
		switch p.matchType {
		case "ua", "path", "rdns":
			re, err := regexp.Compile("(?i)" + p.pattern)
			if err != nil {
				return &policyError{name: d.Name, msg: "正则非法: " + err.Error()}
			}
			p.re = re
		case "ip":
			prefix, err := parsePrefix(p.pattern)
			if err != nil {
				return &policyError{name: d.Name, msg: "IP/CIDR 非法: " + err.Error()}
			}
			p.prefix = prefix
		default:
			return &policyError{name: d.Name, msg: "matchType 只能是 ua/path/ip/rdns"}
		}
		next.policies = append(next.policies, p)
	}
	sort.SliceStable(next.policies, func(i, j int) bool {
		if next.policies[i].priority == next.policies[j].priority {
			return next.policies[i].id < next.policies[j].id
		}
		return next.policies[i].priority < next.policies[j].priority
	})
	for _, h := range honeypots {
		if !h.Enabled || strings.TrimSpace(h.Path) == "" {
			continue
		}
		p := strings.TrimSpace(h.Path)
		next.honeypots = append(next.honeypots, compiledHoneypot{
			site: strings.TrimSpace(h.Site), path: p, exact: !strings.HasSuffix(p, "*"),
		})
	}
	m.state.Store(next)
	m.resolver.setTTL(st.CacheTTL)
	return nil
}

// policyError 是 Bot 配置编译错误。
type policyError struct {
	name string
	msg  string
}

// Error 实现 error。
func (e *policyError) Error() string { return "Bot 策略 " + e.name + " " + e.msg }

// Check 对请求做 Bot 判定。
// 参数 req：请求上下文。
// 返回 Verdict：Matched=false 表示未命中任何 Bot 规则/评分。
func (m *Manager) Check(req *Request) Verdict {
	st := m.state.Load()
	if st == nil || !st.settings.Enabled || req == nil {
		return Verdict{}
	}

	// 1) 蜜罐路径优先级最高：正常用户不会访问，命中直接按恶意处理。
	for _, h := range st.honeypots {
		if h.site != "" && !strings.EqualFold(h.site, req.Site) {
			continue
		}
		if h.exact && req.Path == h.path || !h.exact && strings.HasPrefix(req.Path, strings.TrimSuffix(h.path, "*")) {
			return Verdict{
				Matched: true, Action: ActionBan, Category: "honeypot",
				BotName: "honeypot", Score: 100, Reason: "命中蜜罐路径 " + req.Path,
			}
		}
	}

	// 2) 显式策略：按优先级第一条命中生效。
	for _, p := range st.policies {
		if p.site != "" && !strings.EqualFold(p.site, req.Site) {
			continue
		}
		hit := false
		switch p.matchType {
		case "ua":
			hit = p.re != nil && p.re.MatchString(req.UA)
		case "path":
			hit = p.re != nil && p.re.MatchString(req.Path)
		case "ip":
			hit = p.prefix != nil && req.IP.IsValid() && p.prefix.Contains(req.IP.Unmap())
		case "rdns":
			// rDNS 策略按 PTR 名称做正则匹配；结果带 TTL 缓存，避免每个请求都查 DNS。
			if req.IP.IsValid() && p.re != nil {
				for _, name := range m.resolver.ptr(req.IP) {
					if p.re.MatchString(name) {
						hit = true
						break
					}
				}
			}
		}
		if hit {
			return Verdict{
				Matched: true, Action: p.action, Category: p.category,
				BotName: p.name, Score: 60, Reason: "命中 Bot 策略：" + p.name,
			}
		}
	}

	// 3) 搜索引擎 Bot 验证：真爬虫放行，假爬虫交给行为评分。
	if st.settings.VerifyRDNS && req.IP.IsValid() {
		if name, suffix := botIdentity(req.UA); name != "" {
			if ok, domain := m.resolver.verify(req.IP, suffix); ok {
				return Verdict{
					Matched: true, Action: ActionAllow, Category: "search",
					BotName: name, Verified: true, Score: 0,
					Reason: "已验证搜索引擎 Bot：" + domain,
				}
			}
			// UA 冒充已知爬虫但 rDNS 不通过：直接给高分。
			return Verdict{
				Matched: true, Action: st.settings.Action, Category: "malicious",
				BotName: name, Score: 90, Reason: "伪造搜索引擎 Bot（rDNS 验证失败）",
			}
		}
	}

	// 4) 行为评分：窗口内请求频率 + 可疑 UA。
	count := m.tracker.observe(req.IP.String(), st.settings.Window)
	score := 0
	if count >= st.settings.ScoreThreshold {
		score = 60 + minInt(count-st.settings.ScoreThreshold, 40)
	}
	if suspiciousUA(req.UA) {
		score += 30
	}
	if score >= 60 {
		return Verdict{
			Matched: true, Action: st.settings.Action, Category: "unknown",
			BotName: "unknown", Score: score,
			Reason: "Bot 行为评分超限（窗口请求 " + itoa(count) + " 次）",
		}
	}
	return Verdict{}
}

// Cleanup 清理过期缓存与计数器，由 app 维护循环调用。
// 参数 now：当前时间。
func (m *Manager) Cleanup(now time.Time) {
	m.tracker.cleanup(now)
	m.resolver.cleanup(now)
}

// Stats 返回当前策略/蜜罐数量，供控制面展示。
func (m *Manager) Stats() (policies, honeypots int) {
	st := m.state.Load()
	if st == nil {
		return 0, 0
	}
	return len(st.policies), len(st.honeypots)
}

// Settings 返回当前 Bot 设置快照。
func (m *Manager) Settings() Settings {
	if st := m.state.Load(); st != nil {
		return st.settings
	}
	return Settings{}
}

// tracker 是固定窗口的 IP 请求计数器。
type tracker struct {
	mu     sync.Mutex
	window time.Duration
	items  map[string]*trackItem
}

// trackItem 是单个 IP 的窗口计数。
type trackItem struct {
	start time.Time
	count int
}

// newTracker 创建计数器。
func newTracker(window time.Duration) *tracker {
	return &tracker{window: window, items: make(map[string]*trackItem)}
}

// observe 记录一次请求并返回当前窗口内的累计次数。
func (t *tracker) observe(ip string, window time.Duration) int {
	if window <= 0 {
		window = t.window
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	it := t.items[ip]
	if it == nil || now.Sub(it.start) > window {
		it = &trackItem{start: now}
		t.items[ip] = it
	}
	it.count++
	return it.count
}

// cleanup 删除过期窗口，防止 map 无限增长。
func (t *tracker) cleanup(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, it := range t.items {
		if now.Sub(it.start) > t.window*2 {
			delete(t.items, k)
		}
	}
}

// resolver 是 rDNS 验证器，带结果缓存。
type resolver struct {
	mu       sync.Mutex
	ttl      time.Duration
	cache    map[string]rdnsEntry
	ptrCache map[string]ptrEntry
}

// rdnsEntry 是单次验证结果。
type rdnsEntry struct {
	ok      bool
	domain  string
	expires time.Time
}

// ptrEntry 是单次 PTR 查询结果；空 names 表示查询失败或没有 PTR，同样缓存。
type ptrEntry struct {
	names   []string
	expires time.Time
}

// newResolver 创建验证器。
func newResolver(ttl time.Duration) *resolver {
	return &resolver{
		ttl:      ttl,
		cache:    make(map[string]rdnsEntry),
		ptrCache: make(map[string]ptrEntry),
	}
}

// setTTL 更新缓存时长。
func (r *resolver) setTTL(ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	r.mu.Lock()
	r.ttl = ttl
	r.mu.Unlock()
}

// ptr 查询 IP 的 PTR 名称并按 TTL 缓存，供 rDNS 策略正则匹配。
func (r *resolver) ptr(ip netip.Addr) []string {
	key := "ptr|" + ip.String()
	now := time.Now()
	r.mu.Lock()
	if e, ok := r.ptrCache[key]; ok && now.Before(e.expires) {
		r.mu.Unlock()
		return e.names
	}
	r.mu.Unlock()

	names := lookupPTR(ip)
	r.mu.Lock()
	r.ptrCache[key] = ptrEntry{names: names, expires: now.Add(r.ttl)}
	r.mu.Unlock()
	return names
}

// verify 做正向-反向 DNS 双重验证：PTR 命中断言后缀，且 A/AAAA 能解析回原 IP。
func (r *resolver) verify(ip netip.Addr, suffix string) (bool, string) {
	key := ip.String() + "|" + suffix
	now := time.Now()
	r.mu.Lock()
	if e, ok := r.cache[key]; ok && now.Before(e.expires) {
		r.mu.Unlock()
		return e.ok, e.domain
	}
	r.mu.Unlock()

	ok, domain := checkRDNS(ip, suffix)
	r.mu.Lock()
	r.cache[key] = rdnsEntry{ok: ok, domain: domain, expires: now.Add(r.ttl)}
	r.mu.Unlock()
	return ok, domain
}

// cleanup 清理过期缓存。
func (r *resolver) cleanup(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, e := range r.cache {
		if now.After(e.expires) {
			delete(r.cache, k)
		}
	}
	for k, e := range r.ptrCache {
		if now.After(e.expires) {
			delete(r.ptrCache, k)
		}
	}
}

// lookupPTR 查询 IP 的反向 DNS 名称，统一转小写并去掉末尾的点。
func lookupPTR(ip netip.Addr) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, normalizePTR(name))
	}
	return out
}

// normalizePTR 规范化 PTR 名称。
func normalizePTR(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

// checkRDNS 执行实际的 PTR + 正向解析。
func checkRDNS(ip netip.Addr, suffix string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
	if err != nil {
		return false, ""
	}
	for _, name := range names {
		name = normalizePTR(name)
		if !strings.HasSuffix(name, strings.ToLower(suffix)) {
			continue
		}
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", name)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if a.Unmap() == ip.Unmap() {
				return true, name
			}
		}
	}
	return false, ""
}

// botIdentity 根据 UA 识别已知搜索引擎并返回名称与期望的 rDNS 后缀。
func botIdentity(ua string) (name, suffix string) {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "googlebot"):
		return "Googlebot", ".googlebot.com"
	case strings.Contains(u, "google-inspectiontool"):
		return "Google-InspectionTool", ".google.com"
	case strings.Contains(u, "bingbot"):
		return "Bingbot", ".search.msn.com"
	case strings.Contains(u, "baiduspider"):
		return "Baiduspider", ".baidu.com"
	case strings.Contains(u, "yandexbot"):
		return "YandexBot", ".yandex.com"
	case strings.Contains(u, "applebot"):
		return "Applebot", ".applebot.apple.com"
	case strings.Contains(u, "petalbot"):
		return "PetalBot", ".petalsearch.com"
	}
	return "", ""
}

// suspiciousUA 判断 UA 是否像脚本/自动化工具。
func suspiciousUA(ua string) bool {
	u := strings.ToLower(strings.TrimSpace(ua))
	if u == "" {
		return true
	}
	for _, s := range []string{
		"python-requests", "python-urllib", "curl/", "wget/", "go-http-client",
		"scrapy", "headlesschrome", "phantomjs", "selenium", "puppeteer",
		"playwright", "httpclient", "okhttp", "java/", "libwww-perl",
	} {
		if strings.Contains(u, s) {
			return true
		}
	}
	return false
}

// parsePrefix 解析 CIDR；单个 IP 自动补 /32 或 /128。
func parsePrefix(v string) (*netip.Prefix, error) {
	v = strings.TrimSpace(v)
	if p, err := netip.ParsePrefix(v); err == nil {
		p = p.Masked()
		return &p, nil
	}
	addr, err := netip.ParseAddr(v)
	if err != nil {
		return nil, err
	}
	addr = addr.Unmap()
	bits := 128
	if addr.Is4() {
		bits = 32
	}
	p := netip.PrefixFrom(addr, bits)
	return &p, nil
}

// minInt 返回较小值。
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// itoa 是无依赖的整数转字符串，避免在热路径引入 fmt。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// 编译期确认 net/http 仅用于 Header 类型占位，避免误删导入。
var _ = http.Header{}
