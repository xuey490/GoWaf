package guard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"
)

// limiterShards 是限流器内部分片数量，取 2 的幂有利于哈希均匀散列；
// 分片各自持锁，降低高并发下的锁竞争。
const limiterShards = 16

// bucket 是单个 IP 的限流状态。
type bucket struct {
	tokens      float64 // 当前令牌数，随时间按 rate 补充，上限为 burst
	lastMS      int64   // 上次补充令牌的毫秒时间戳
	minute      int64   // 当前分钟桶编号（Unix 毫秒时间戳 / 60000）
	minuteCount int     // 当前分钟内已放行的请求数
}

// limiterShard 是限流器的一个分片，独立加锁以降低竞争。
type limiterShard struct {
	mu sync.Mutex         // 保护本分片 map 的互斥锁
	m  map[string]*bucket // 键为客户端 IP，值为该 IP 的限流状态
}

// Limiter 令牌桶 + 分钟计数双闸门。
//
// 令牌桶管"突发"，分钟计数管"持续低频刷"。两者都要有：
// 只有令牌桶时，攻击者把速度降到 rate 以下就能无限刷；
// 只有分钟计数时，突发会把后端打出一个尖峰。
type Limiter struct {
	rate      float64                     // 令牌补充速率：每秒补充的令牌数
	burst     float64                     // 令牌桶容量：允许的最大突发量
	perMinute int                         // 每分钟最大放行数，<=0 表示不启用分钟限制
	shards    [limiterShards]limiterShard // 按 IP 哈希取模分片，降低锁竞争
}

// NewLimiter 创建限流器。
// 参数 rate：每秒补充的令牌数（<=0 时取默认值 30）；参数 burst：令牌桶容量（<=0 时取默认值 60）；
// 参数 perMinute：每分钟最大放行数（<=0 表示不启用分钟限制）。
// 返回 *Limiter：初始化完成的限流器，全部分片 map 就绪。
func NewLimiter(rate, burst, perMinute int) *Limiter {
	if rate <= 0 {
		rate = 30
	}
	if burst <= 0 {
		burst = 60
	}
	l := &Limiter{
		rate:      float64(rate),
		burst:     float64(burst),
		perMinute: perMinute,
	}
	for i := range l.shards {
		l.shards[i].m = make(map[string]*bucket)
	}
	return l
}

// shard 按 IP 哈希取模定位所属分片。
// 参数 ip：客户端 IP 字符串。
// 返回 *limiterShard：负责该 IP 的分片，同一 IP 永远落在同一分片。
func (l *Limiter) shard(ip string) *limiterShard {
	return &l.shards[fnv(ip)%limiterShards]
}

// fnv 简易哈希，足够分片用。
func fnv(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// Reason 限流原因。
type Reason string

// 限流原因常量，作为 Allow 的拒绝原因返回，用于日志与响应提示。
const (
	ReasonNone   Reason = ""       // 放行，未触发限流
	ReasonRate   Reason = "rate"   // 令牌桶耗尽：突发速率超限
	ReasonMinute Reason = "minute" // 分钟计数超限：持续请求过多
)

// Allow 判断请求是否放行，并扣减对应配额。
// 参数 ip：客户端 IP 字符串；参数 nowMS：当前毫秒时间戳。
// 返回 bool：是否放行；返回 Reason：拒绝原因，放行时为 ReasonNone。
func (l *Limiter) Allow(ip string, nowMS int64) (bool, Reason) {
	sh := l.shard(ip)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	b, ok := sh.m[ip]
	if !ok {
		// 首次出现的 IP：满桶令牌起步，分钟计数从当前分钟开始
		b = &bucket{tokens: l.burst, lastMS: nowMS, minute: nowMS / 60000}
		sh.m[ip] = b
	}

	// 分钟桶滚动
	minute := nowMS / 60000
	if b.minute != minute {
		b.minute = minute
		b.minuteCount = 0
	}

	// 令牌补充
	elapsed := float64(nowMS-b.lastMS) / 1000.0
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastMS = nowMS
	}

	if b.tokens < 1 {
		return false, ReasonRate // 令牌不足：突发超限，直接拒绝且不消耗分钟计数
	}
	// 先扣减再判断：即使本次因分钟超限被拒，令牌与分钟计数也已消耗
	b.tokens--
	b.minuteCount++
	if l.perMinute > 0 && b.minuteCount > l.perMinute {
		return false, ReasonMinute
	}
	return true, ReasonNone
}

// Cleanup 清理长时间不活跃的条目，防止 map 无限增长（内存泄漏的主要来源）。
// 参数 nowMS：当前毫秒时间戳。
// 返回 int：删除的条目数。最后活跃时间早于 10 分钟前的 IP 会被移除，下次请求按满桶重新起步。
func (l *Limiter) Cleanup(nowMS int64) int {
	removed := 0
	cutoff := nowMS - 10*60*1000
	for i := range l.shards {
		sh := &l.shards[i]
		sh.mu.Lock()
		for k, b := range sh.m {
			if b.lastMS < cutoff {
				delete(sh.m, k)
				removed++
			}
		}
		sh.mu.Unlock()
	}
	return removed
}

// Size 返回当前跟踪的 IP 数。
// 返回 int：全部分片 map 的条目总数。
func (l *Limiter) Size() int {
	n := 0
	for i := range l.shards {
		sh := &l.shards[i]
		sh.mu.Lock()
		n += len(sh.m)
		sh.mu.Unlock()
	}
	return n
}

// HitTracker 统计"命中拦截规则"的频次，用于自动封禁。
//
// 这里只负责"窗口内打了几次"，不负责"累计被封了几次"——
// 连击数必须跨重启存活，所以以 bans 表为准（见 edge.autoBanStrikes）。
type HitTracker struct {
	mu sync.Mutex           // 保护 m 的互斥锁
	m  map[string]*hitState // 键为客户端 IP，值为该 IP 的窗口计数状态
}

// hitState 是单个 IP 的窗口计数状态。
type hitState struct {
	windowStart int64 // 当前窗口的起始毫秒时间戳
	count       int   // 窗口内累计命中次数
}

// NewHitTracker 创建命中追踪器。
// 返回 *HitTracker：初始化完成的追踪器。
func NewHitTracker() *HitTracker {
	return &HitTracker{m: make(map[string]*hitState)}
}

// Add 记录一次命中，返回窗口内累计次数与是否达到封禁阈值。
// 参数 ip：客户端 IP；参数 nowMS：当前毫秒时间戳；参数 windowMS：窗口时长（毫秒）；
// 参数 threshold：封禁阈值（<=0 表示关闭检测，直接返回不计数）。
// 返回 count：窗口内累计次数（触发时返回阈值本身）；返回 reached：是否达到封禁阈值。
func (t *HitTracker) Add(ip string, nowMS, windowMS int64, threshold int) (count int, reached bool) {
	if threshold <= 0 {
		return 0, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	st, ok := t.m[ip]
	if !ok {
		st = &hitState{windowStart: nowMS}
		t.m[ip] = st
	}
	if nowMS-st.windowStart > windowMS {
		// 超出窗口时长：重开窗口，从本次重新累计
		st.windowStart = nowMS
		st.count = 0
	}
	st.count++
	if st.count >= threshold {
		// 达到阈值：归零并重开窗口，避免同一波连击反复触发封禁
		st.count = 0
		st.windowStart = nowMS
		return threshold, true
	}
	return st.count, false
}

// WindowHits 返回该 IP 当前窗口内的命中数（只读，用于面板展示）。
// 参数 ip：客户端 IP。
// 返回 int：窗口内命中次数；无记录时返回 0。
func (t *HitTracker) WindowHits(ip string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if st, ok := t.m[ip]; ok {
		return st.count
	}
	return 0
}

// Reset 清空某 IP 的窗口计数（手动解封时调用，给一次干净的开始）。
// 参数 ip：客户端 IP；无记录时为空操作。
func (t *HitTracker) Reset(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, ip)
}

// Cleanup 清理长时间无命中的记录，防止 map 无限增长。
// 参数 nowMS：当前毫秒时间戳；参数 idleMS：闲置阈值（毫秒），窗口起始早于 nowMS-idleMS 的记录被删除。
// 返回 int：删除的记录数。
func (t *HitTracker) Cleanup(nowMS, idleMS int64) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	removed := 0
	for k, st := range t.m {
		if nowMS-st.windowStart > idleMS {
			delete(t.m, k)
			removed++
		}
	}
	return removed
}

// ===== 人机校验（JS Challenge）=====

// Challenger 用 HMAC 签名下发一个短期通行证，避免引入第三方验证码。
type Challenger struct {
	secret   []byte        // HMAC-SHA256 签名密钥
	ttl      time.Duration // 通行证有效期
	cookie   string        // 通行证写入的 Cookie 名
	passPath string        // 人机校验页面的路径
}

// NewChallenger 创建人机校验器。
// 参数 secret：HMAC 签名密钥；参数 ttl：通行证有效期（<=0 时取默认值 30 分钟）。
// 返回 *Challenger：初始化完成的校验器。
func NewChallenger(secret []byte, ttl time.Duration) *Challenger {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &Challenger{secret: secret, ttl: ttl, cookie: "waf_pass", passPath: "/__waf/challenge"}
}

// CookieName 返回通行证 Cookie 名，供下发与清除 Cookie 时使用。
// 返回 string：固定为 "waf_pass"。
func (c *Challenger) CookieName() string { return c.cookie }

// PassPath 返回人机校验页路径。
// 返回 string：固定为 "/__waf/challenge"。
func (c *Challenger) PassPath() string { return c.passPath }

// Sign 为 IP 生成签名通行证。
// 参数 ip：通过校验的客户端 IP；参数 now：当前时间，用于计算过期时刻。
// 返回 string：形如 base64(payload)+"."+base64(签名) 的令牌，payload 为 "ip|过期毫秒时间戳"。
func (c *Challenger) Sign(ip string, now time.Time) string {
	// 过期时刻 = 当前时间 + 有效期，与 IP 一起拼入 payload 并整体签名
	exp := now.Add(c.ttl).UnixMilli()
	payload := ip + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify 校验通行证是否有效。
// 参数 token：客户端提交的令牌；参数 ip：当前请求的客户端 IP；参数 now：当前时间。
// 返回 bool：签名正确、payload 中的 IP 与当前 IP 一致且未过期时返回 true，任一条件不满足返回 false。
func (c *Challenger) Verify(token, ip string, now time.Time) bool {
	// 令牌格式：payload 与签名以 '.' 分隔
	dot := strings.LastIndexByte(token, '.')
	if dot <= 0 {
		return false
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(token[dot+1:])
	if err != nil {
		return false
	}
	// 先验签再解析内容，防止伪造 payload
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(rawPayload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return false
	}
	// payload 为 "ip|过期毫秒时间戳"，IP 必须与当前请求一致
	parts := strings.SplitN(string(rawPayload), "|", 2)
	if len(parts) != 2 || parts[0] != ip {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	return now.UnixMilli() < exp // 当前时间早于过期时刻才有效
}
