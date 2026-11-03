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

// limiterShards 分片数，2 的幂，减少锁竞争。
const limiterShards = 16

// bucket 是单个 IP 的限流状态。
type bucket struct {
	tokens      float64
	lastMS      int64
	minute      int64
	minuteCount int
}

type limiterShard struct {
	mu sync.Mutex
	m  map[string]*bucket
}

// Limiter 令牌桶 + 分钟计数双闸门。
//
// 令牌桶管"突发"，分钟计数管"持续低频刷"。两者都要有：
// 只有令牌桶时，攻击者把速度降到 rate 以下就能无限刷；
// 只有分钟计数时，突发会把后端打出一个尖峰。
type Limiter struct {
	rate      float64
	burst     float64
	perMinute int
	shards    [limiterShards]limiterShard
}

// NewLimiter 创建限流器。
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

// 限流原因常量。
const (
	ReasonNone   Reason = ""
	ReasonRate   Reason = "rate"
	ReasonMinute Reason = "minute"
)

// Allow 判断是否放行。
func (l *Limiter) Allow(ip string, nowMS int64) (bool, Reason) {
	sh := l.shard(ip)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	b, ok := sh.m[ip]
	if !ok {
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
		return false, ReasonRate
	}
	b.tokens--
	b.minuteCount++
	if l.perMinute > 0 && b.minuteCount > l.perMinute {
		return false, ReasonMinute
	}
	return true, ReasonNone
}

// Cleanup 清理长时间不活跃的条目，防止 map 无限增长（内存泄漏的主要来源）。
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
	mu sync.Mutex
	m  map[string]*hitState
}

type hitState struct {
	windowStart int64
	count       int
}

// NewHitTracker 创建追踪器。
func NewHitTracker() *HitTracker {
	return &HitTracker{m: make(map[string]*hitState)}
}

// Add 记录一次命中，返回窗口内累计次数与是否达到封禁阈值。
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
		st.windowStart = nowMS
		st.count = 0
	}
	st.count++
	if st.count >= threshold {
		st.count = 0
		st.windowStart = nowMS
		return threshold, true
	}
	return st.count, false
}

// WindowHits 返回该 IP 当前窗口内的命中数（只读，用于面板展示）。
func (t *HitTracker) WindowHits(ip string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if st, ok := t.m[ip]; ok {
		return st.count
	}
	return 0
}

// Reset 清空某 IP 的窗口计数（手动解封时调用，给一次干净的开始）。
func (t *HitTracker) Reset(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, ip)
}

// Cleanup 清理长时间无命中的记录。
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
	secret   []byte
	ttl      time.Duration
	cookie   string
	passPath string
}

// NewChallenger 创建校验器。
func NewChallenger(secret []byte, ttl time.Duration) *Challenger {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &Challenger{secret: secret, ttl: ttl, cookie: "waf_pass", passPath: "/__waf/challenge"}
}

// CookieName 返回通行证 Cookie 名。
func (c *Challenger) CookieName() string { return c.cookie }

// PassPath 返回校验页路径。
func (c *Challenger) PassPath() string { return c.passPath }

// Sign 为 IP 生成签名通行证。
func (c *Challenger) Sign(ip string, now time.Time) string {
	exp := now.Add(c.ttl).UnixMilli()
	payload := ip + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify 校验通行证是否有效。
func (c *Challenger) Verify(token, ip string, now time.Time) bool {
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
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(rawPayload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return false
	}
	parts := strings.SplitN(string(rawPayload), "|", 2)
	if len(parts) != 2 || parts[0] != ip {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	return now.UnixMilli() < exp
}
