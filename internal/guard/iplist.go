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

// IPList 是不可变的 IP 名单快照。
type IPList struct {
	whiteExact map[netip.Addr]struct{}
	blackExact map[netip.Addr]struct{}
	whiteNet   []netip.Prefix
	blackNet   []netip.Prefix
}

// BuildIPList 从数据库记录构建快照，自动过滤已过期的黑名单。
func BuildIPList(rows []model.IPRule, nowMS int64) *IPList {
	l := &IPList{
		whiteExact: map[netip.Addr]struct{}{},
		blackExact: map[netip.Addr]struct{}{},
	}
	for _, r := range rows {
		if r.ExpiresAt > 0 && r.ExpiresAt < nowMS && r.Kind == "black" {
			continue
		}
		isWhite := r.Kind == "white"
		if p, err := netip.ParsePrefix(r.IP); err == nil {
			if isWhite {
				l.whiteNet = append(l.whiteNet, p)
			} else {
				l.blackNet = append(l.blackNet, p)
			}
			continue
		}
		addr, err := netip.ParseAddr(r.IP)
		if err != nil {
			continue
		}
		if isWhite {
			l.whiteExact[addr] = struct{}{}
		} else {
			l.blackExact[addr] = struct{}{}
		}
	}
	return l
}

// IsWhitelisted 白名单优先，白名单内的 IP 不做任何检测。
func (l *IPList) IsWhitelisted(addr netip.Addr) bool {
	if l == nil {
		return false
	}
	if _, ok := l.whiteExact[addr]; ok {
		return true
	}
	for _, p := range l.whiteNet {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// IsBlacklisted 判断是否黑名单。
func (l *IPList) IsBlacklisted(addr netip.Addr) bool {
	if l == nil {
		return false
	}
	if _, ok := l.blackExact[addr]; ok {
		return true
	}
	for _, p := range l.blackNet {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Count 返回名单规模，供面板展示。
func (l *IPList) Count() (white, black int) {
	if l == nil {
		return 0, 0
	}
	return len(l.whiteExact) + len(l.whiteNet), len(l.blackExact) + len(l.blackNet)
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
type BanList struct {
	mu    chan struct{} // 用容量 1 的 channel 当锁，避免惰性初始化问题
	items atomic.Pointer[map[string]int64]
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

// Banned 判断是否在封禁中。
func (b *BanList) Banned(ip string, nowMS int64) bool {
	m := *b.items.Load()
	until, ok := m[ip]
	if !ok {
		return false
	}
	if until > 0 && until < nowMS {
		return false // 已过期，等清理协程回收
	}
	return true
}

// Ban 写入封禁，until 为毫秒时间戳（0 表示永久）。
func (b *BanList) Ban(ip string, until int64) {
	b.lock()
	defer b.unlock()
	old := *b.items.Load()
	next := make(map[string]int64, len(old)+1)
	for k, v := range old {
		next[k] = v
	}
	next[ip] = until
	b.items.Store(&next)
}

// Unban 解除封禁。
func (b *BanList) Unban(ip string) bool {
	b.lock()
	defer b.unlock()
	old := *b.items.Load()
	if _, ok := old[ip]; !ok {
		return false
	}
	next := make(map[string]int64, len(old))
	for k, v := range old {
		if k == ip {
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
		next[r.IP] = r.ExpiresAt
	}
	b.lock()
	b.items.Store(&next)
	b.unlock()
}

// Snapshot 返回当前封禁副本。
func (b *BanList) Snapshot() map[string]int64 {
	m := *b.items.Load()
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Size 返回封禁数量。
func (b *BanList) Size() int { return len(*b.items.Load()) }

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
