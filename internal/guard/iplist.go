// Package guard 提供 IP 名单、限流、自动封禁与人机校验。
//
// 所有结构体构建后以原子快照方式被数据面读取，避免每请求加锁。
package guard

import (
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"gowaf/internal/store/model"
)

// ipSet 是单一作用域（全局或某个站点）下的 IP 集合。
type ipSet struct {
	whiteExact map[netip.Addr]struct{}
	blackExact map[netip.Addr]struct{}
	whiteNet   []netip.Prefix
	blackNet   []netip.Prefix
}

func newIPSet() *ipSet {
	return &ipSet{
		whiteExact: map[netip.Addr]struct{}{},
		blackExact: map[netip.Addr]struct{}{},
	}
}

func (s *ipSet) add(addr netip.Addr, isWhite bool) {
	if isWhite {
		s.whiteExact[addr] = struct{}{}
		return
	}
	s.blackExact[addr] = struct{}{}
}

func (s *ipSet) white(addr netip.Addr) bool {
	if s == nil {
		return false
	}
	if _, ok := s.whiteExact[addr]; ok {
		return true
	}
	for _, p := range s.whiteNet {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func (s *ipSet) black(addr netip.Addr) bool {
	if s == nil {
		return false
	}
	if _, ok := s.blackExact[addr]; ok {
		return true
	}
	for _, p := range s.blackNet {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// IPList 是不可变的 IP 名单快照。
//
// 全局名单对所有站点生效；站点名单只在该站点命中。
// 优先级：站点名单 > 全局名单（白名单始终优先于黑名单）。
type IPList struct {
	global ipSet
	sites  map[string]*ipSet
}

// BuildIPList 从数据库记录构建快照，自动过滤已过期的黑名单。
func BuildIPList(rows []model.IPRule, nowMS int64) *IPList {
	l := &IPList{
		global: *newIPSet(),
		sites:  map[string]*ipSet{},
	}
	for _, r := range rows {
		if r.ExpiresAt > 0 && r.ExpiresAt < nowMS && r.Kind == "black" {
			continue
		}
		isWhite := r.Kind == "white"
		target := &l.global
		// scope=site 但没填站点属于脏数据，按全局处理会误伤，直接忽略这条更安全
		if r.Scope == "site" {
			if r.Site == "" {
				continue
			}
			set, ok := l.sites[r.Site]
			if !ok {
				set = newIPSet()
				l.sites[r.Site] = set
			}
			target = set
		}
		if p, err := netip.ParsePrefix(r.IP); err == nil {
			if isWhite {
				target.whiteNet = append(target.whiteNet, p)
			} else {
				target.blackNet = append(target.blackNet, p)
			}
			continue
		}
		addr, err := netip.ParseAddr(r.IP)
		if err != nil {
			continue
		}
		target.add(addr, isWhite)
	}
	return l
}

// IsWhitelisted 白名单优先，白名单内的 IP 不做任何检测。
// site 为当前请求命中的站点名；站点名单只对该站点生效。
func (l *IPList) IsWhitelisted(addr netip.Addr, site string) bool {
	if l == nil {
		return false
	}
	if s := l.sites[site]; s != nil && s.white(addr) {
		return true
	}
	return l.global.white(addr)
}

// IsBlacklisted 判断是否黑名单。
func (l *IPList) IsBlacklisted(addr netip.Addr, site string) bool {
	if l == nil {
		return false
	}
	if s := l.sites[site]; s != nil && s.black(addr) {
		return true
	}
	return l.global.black(addr)
}

// Count 返回名单规模，供面板展示。
func (l *IPList) Count() (white, black int) {
	if l == nil {
		return 0, 0
	}
	sets := []*ipSet{&l.global}
	for _, s := range l.sites {
		sets = append(sets, s)
	}
	for _, s := range sets {
		white += len(s.whiteExact) + len(s.whiteNet)
		black += len(s.blackExact) + len(s.blackNet)
	}
	return white, black
}

// Holder 用 atomic.Pointer 承载名单快照。
type Holder struct {
	p atomic.Pointer[IPList]
}

// Store 原子替换快照。
func (h *Holder) Store(l *IPList) { h.p.Store(l) }

// Load 读取当前快照。
func (h *Holder) Load() *IPList { return h.p.Load() }

// BanList 内存封禁表，与数据库最终一致（重启后由 LoadFromDB 恢复）。
//
// key 由「作用域站点 + IP」拼成：全局封禁用裸 IP，站点封禁用 site+"\x00"+IP。
// 站点名为空串即全局；站点名不含 \x00，拼接无歧义。
type BanList struct {
	mu    chan struct{} // 用容量 1 的 channel 当锁，避免惰性初始化问题
	items atomic.Pointer[map[string]int64]
}

// banKey 生成封禁表键。site 为空表示全局封禁。
func banKey(site, ip string) string {
	if site == "" {
		return ip
	}
	return site + "\x00" + ip
}

// NewBanList 创建封禁表。
func NewBanList() *BanList {
	b := &BanList{mu: make(chan struct{}, 1)}
	b.mu <- struct{}{}
	empty := map[string]int64{}
	b.items.Store(&empty)
	return b
}

func (b *BanList) lock()   { <-b.mu }
func (b *BanList) unlock() { b.mu <- struct{}{} }

// Banned 判断是否在封禁中：全局封禁或该站点封禁任一命中即视为封禁。
func (b *BanList) Banned(ip, site string, nowMS int64) bool {
	m := *b.items.Load()
	if until, ok := m[banKey("", ip)]; ok && (until == 0 || until > nowMS) {
		return true
	}
	if site == "" {
		return false
	}
	until, ok := m[banKey(site, ip)]
	if !ok {
		return false
	}
	if until > 0 && until < nowMS {
		return false // 已过期，等清理协程回收
	}
	return true
}

// Ban 写入封禁，until 为毫秒时间戳（0 表示永久）。site 为空表示全局封禁。
func (b *BanList) Ban(ip, site string, until int64) {
	b.lock()
	defer b.unlock()
	old := *b.items.Load()
	next := make(map[string]int64, len(old)+1)
	for k, v := range old {
		next[k] = v
	}
	next[banKey(site, ip)] = until
	b.items.Store(&next)
}

// Unban 解除指定作用域的封禁。
func (b *BanList) Unban(ip, site string) bool {
	key := banKey(site, ip)
	b.lock()
	defer b.unlock()
	old := *b.items.Load()
	if _, ok := old[key]; !ok {
		return false
	}
	next := make(map[string]int64, len(old))
	for k, v := range old {
		if k == key {
			continue
		}
		next[k] = v
	}
	b.items.Store(&next)
	return true
}

// Clear 清空全部封禁（面板上的"panic 按钮"）。
func (b *BanList) Clear() int {
	b.lock()
	defer b.unlock()
	n := len(*b.items.Load())
	empty := map[string]int64{}
	b.items.Store(&empty)
	return n
}

// LoadFromDB 用数据库记录重建。
func (b *BanList) LoadFromDB(rows []model.Ban, nowMS int64) {
	next := make(map[string]int64, len(rows))
	for _, r := range rows {
		if r.ReleasedAt > 0 {
			continue
		}
		if r.ExpiresAt > 0 && r.ExpiresAt < nowMS {
			continue
		}
		next[banKey(banSite(r), r.IP)] = r.ExpiresAt
	}
	b.lock()
	b.items.Store(&next)
	b.unlock()
}

// banSite 取记录实际生效的站点：只有 scope=site 才带站点，脏数据按全局处理。
func banSite(r model.Ban) string {
	if r.Scope == "site" {
		return r.Site
	}
	return ""
}

// Snapshot 返回当前封禁副本。key 为内部键：全局是裸 IP，站点封禁为 site+"\x00"+IP。
func (b *BanList) Snapshot() map[string]int64 {
	m := *b.items.Load()
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Size 返回被封禁的 IP 数量（同一 IP 的全局与站点封禁只算一个）。
func (b *BanList) Size() int {
	m := *b.items.Load()
	seen := make(map[string]struct{}, len(m))
	for k := range m {
		ip := k
		if i := strings.IndexByte(k, 0); i >= 0 {
			ip = k[i+1:]
		}
		seen[ip] = struct{}{}
	}
	return len(seen)
}

// Cleanup 清理过期条目，返回清理数量。
func (b *BanList) Cleanup(nowMS int64) int {
	b.lock()
	defer b.unlock()
	old := *b.items.Load()
	next := make(map[string]int64, len(old))
	removed := 0
	for k, v := range old {
		if v > 0 && v < nowMS {
			removed++
			continue
		}
		next[k] = v
	}
	if removed > 0 {
		b.items.Store(&next)
	}
	return removed
}

// ParseIP 解析客户端 IP，兼容 "1.2.3.4:5678" 与 IPv6。
func ParseIP(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// Now 便于单测替换。
var Now = time.Now
