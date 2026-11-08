// Package guard 提供 IP 名单（白名单/黑名单）、限流、自动封禁、人机校验，
// 以及 Cloudflare 回源网段判定等访问防护能力（名单与封禁见本文件，
// 限流与人机校验见 limiter.go，Cloudflare 网段见 cloudflare.go）。
//
// 核心数据结构构建后以原子快照方式被数据面读取：读路径无锁，
// 写路径整体替换快照，避免每请求加锁。
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
	whiteExact map[netip.Addr]struct{} // 精确 IP 白名单
	blackExact map[netip.Addr]struct{} // 精确 IP 黑名单
	whiteNet   []netip.Prefix          // CIDR 形式的白名单网段
	blackNet   []netip.Prefix          // CIDR 形式的黑名单网段
}

// newIPSet 创建空的 IP 集合。
// 返回 *ipSet：精确匹配的两个 map 已初始化，CIDR 切片按需追加。
func newIPSet() *ipSet {
	return &ipSet{
		whiteExact: map[netip.Addr]struct{}{},
		blackExact: map[netip.Addr]struct{}{},
	}
}

// add 写入一条精确 IP 条目。
// 参数 addr：已解析的 IP；参数 isWhite：true 写入白名单，false 写入黑名单。
func (s *ipSet) add(addr netip.Addr, isWhite bool) {
	if isWhite {
		s.whiteExact[addr] = struct{}{}
		return
	}
	s.blackExact[addr] = struct{}{}
}

// white 判断是否命中白名单：先查精确 map，再遍历 CIDR 网段。
// 参数 addr：待判定 IP。
// 返回 bool：精确命中或任一网段覆盖返回 true；接收者为 nil 或未命中返回 false。
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

// black 判断是否命中黑名单：先查精确 map，再遍历 CIDR 网段。
// 参数 addr：待判定 IP。
// 返回 bool：精确命中或任一网段覆盖返回 true；接收者为 nil 或未命中返回 false。
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
// 优先级：站点名单 > 全局名单；白名单 > 黑名单。白名单是「完全豁免」，
// 数据面第一步即判定，命中后黑名单、内存封禁、威胁情报、限流与规则引擎全部不再参与。
type IPList struct {
	global ipSet             // 全局名单，对所有站点生效
	sites  map[string]*ipSet // 站点名单，键为站点名，只在该站点命中
}

// BuildIPList 从数据库记录构建不可变名单快照。
// 参数 rows：数据库中的 IP 规则记录；参数 nowMS：当前毫秒时间戳，用于判断过期。
// 返回 *IPList：构建好的快照；非法 IP、scope=site 但缺站点的脏数据记录会被跳过。
func BuildIPList(rows []model.IPRule, nowMS int64) *IPList {
	l := &IPList{
		global: *newIPSet(),
		sites:  map[string]*ipSet{},
	}
	for _, r := range rows {
		// 黑白名单都受过期时间约束：带过期时间且已过期的条目直接丢弃。
		// 白名单过期尤其重要——「临时放行某台机器 2 小时」到期后必须重新参与检测，
		// 否则一条带到期时间的白名单会悄悄变成永久放行。
		if r.ExpiresAt > 0 && r.ExpiresAt < nowMS {
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
		// 先按 CIDR 解析，失败再按单 IP 解析；两者都失败视为脏数据跳过
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

// IsWhitelisted 判断 IP 是否在白名单内，白名单优先级最高，命中后不做任何检测。
// 参数 addr：待判定 IP；参数 site：当前请求命中的站点名，站点名单只对该站点生效。
// 返回 bool：站点白名单或全局白名单任一命中返回 true；接收者为 nil 返回 false。
//
// 调用方（edge.ServeHTTP）必须在黑名单与内存封禁之前调用本方法，
// 否则「先被封禁、后加白名单」的 IP 会继续吃 403，与白名单语义矛盾。
func (l *IPList) IsWhitelisted(addr netip.Addr, site string) bool {
	if l == nil {
		return false
	}
	if s := l.sites[site]; s != nil && s.white(addr) {
		return true
	}
	return l.global.white(addr)
}

// IsWhitelistedAnywhere 判断 IP 是否命中任意作用域的白名单（全局或任一站点）。
//
// 用于「全局封禁」这类会波及所有站点的管理动作：只要某个站点把它当自己人，
// 全局封禁就不该落在它头上。全局封禁没有站点参数，所以不能只查全局名单。
// 参数 addr：待判定 IP。
// 返回 bool：命中全局白名单或任一站点的白名单返回 true；接收者为 nil 返回 false。
func (l *IPList) IsWhitelistedAnywhere(addr netip.Addr) bool {
	if l == nil {
		return false
	}
	if l.global.white(addr) {
		return true
	}
	for _, s := range l.sites {
		if s.white(addr) {
			return true
		}
	}
	return false
}

// IsBlacklisted 判断 IP 是否在黑名单内。
// 参数 addr：待判定 IP；参数 site：当前请求命中的站点名，站点名单只对该站点生效。
// 返回 bool：站点黑名单或全局黑名单任一命中返回 true；接收者为 nil 返回 false。
func (l *IPList) IsBlacklisted(addr netip.Addr, site string) bool {
	if l == nil {
		return false
	}
	if s := l.sites[site]; s != nil && s.black(addr) {
		return true
	}
	return l.global.black(addr)
}

// Count 统计名单规模，供面板展示。
// 返回 white：白名单条目数（精确 IP 与 CIDR 分别计数）；返回 black：黑名单条目数。
// 统计范围为全局名单与全部站点名单。
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

// Holder 用 atomic.Pointer 承载名单快照，后台重建后整体替换，数据面读路径无锁。
type Holder struct {
	p atomic.Pointer[IPList] // 当前生效的名单快照
}

// Store 原子替换当前快照，新名单立即对后续读请求生效。
// 参数 l：新构建的名单快照。
func (h *Holder) Store(l *IPList) { h.p.Store(l) }

// Load 读取当前生效的名单快照。
// 返回 *IPList：当前快照；尚未 Store 过时返回 nil。
func (h *Holder) Load() *IPList { return h.p.Load() }

// BanList 内存封禁表，与数据库最终一致（重启后由 LoadFromDB 恢复）。
//
// key 由「作用域站点 + IP」拼成：全局封禁用裸 IP，站点封禁用 site+"\x00"+IP。
// 站点名为空串即全局；站点名不含 \x00，拼接无歧义。
type BanList struct {
	mu    chan struct{}                    // 用容量 1 的 channel 当锁，避免惰性初始化问题
	items atomic.Pointer[map[string]int64] // 封禁表快照：键为 banKey 的结果，值为封禁截止毫秒时间戳（0 表示永久）
}

// banKey 生成封禁表键。
// 参数 site：作用域站点名，空串表示全局封禁；参数 ip：被封禁的 IP 字符串。
// 返回 string：全局封禁为裸 IP；站点封禁为 site+"\x00"+ip（站点名不含 \x00，拼接无歧义）。
func banKey(site, ip string) string {
	if site == "" {
		return ip
	}
	return site + "\x00" + ip
}

// NewBanList 创建封禁表。
// 返回 *BanList：锁与空快照均已初始化、可直接使用的封禁表（零值不可用）。
func NewBanList() *BanList {
	b := &BanList{mu: make(chan struct{}, 1)}
	b.mu <- struct{}{} // 预置令牌：锁初始为空闲状态
	empty := map[string]int64{}
	b.items.Store(&empty)
	return b
}

// lock 获取写锁：从 channel 取走令牌即表示持有锁。
// unlock 释放写锁：把令牌放回 channel。二者必须配对使用（通常配 defer unlock）。
func (b *BanList) lock()   { <-b.mu }
func (b *BanList) unlock() { b.mu <- struct{}{} }

// Banned 判断 IP 当前是否处于封禁状态。
// 参数 ip：客户端 IP 字符串；参数 site：当前站点名（空串时只查全局封禁）；参数 nowMS：当前毫秒时间戳。
// 返回 bool：全局封禁或该站点封禁任一命中且未过期返回 true。
func (b *BanList) Banned(ip, site string, nowMS int64) bool {
	m := *b.items.Load()
	// 全局封禁：截止时间为 0 表示永久，否则未过期即命中
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

// Ban 写入一条封禁。
// 参数 ip：客户端 IP 字符串；参数 site：作用域站点名（空串表示全局封禁）；
// 参数 until：封禁截止毫秒时间戳（0 表示永久）。
// 写入采用「复制旧表后整体替换快照」的方式，读路径不受影响。
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
// 参数 ip：客户端 IP 字符串；参数 site：作用域站点名（空串表示解除全局封禁）。
// 返回 bool：条目存在并已解除返回 true；不存在返回 false。
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
// 返回 int：清空前的封禁条目数。
func (b *BanList) Clear() int {
	b.lock()
	defer b.unlock()
	n := len(*b.items.Load())
	empty := map[string]int64{}
	b.items.Store(&empty)
	return n
}

// LoadFromDB 用数据库记录整体重建封禁表（启动恢复或同步时调用）。
// 参数 rows：数据库中的封禁记录；参数 nowMS：当前毫秒时间戳，用于过滤已过期记录。
func (b *BanList) LoadFromDB(rows []model.Ban, nowMS int64) {
	next := make(map[string]int64, len(rows))
	for _, r := range rows {
		if r.ReleasedAt > 0 { // 已手动解封的记录不再恢复
			continue
		}
		if r.ExpiresAt > 0 && r.ExpiresAt < nowMS { // 已过期的记录不再恢复
			continue
		}
		next[banKey(banSite(r), r.IP)] = r.ExpiresAt
	}
	b.lock()
	b.items.Store(&next)
	b.unlock()
}

// banSite 取记录实际生效的站点作用域。
// 参数 r：数据库封禁记录。
// 返回 string：scope=site 时返回记录中的站点名；其余情况（含脏数据）返回空串表示全局。
func banSite(r model.Ban) string {
	if r.Scope == "site" {
		return r.Site
	}
	return ""
}

// Snapshot 返回当前封禁表的副本，供面板展示或遍历。
// 返回 map[string]int64：键为内部键（全局是裸 IP，站点封禁为 site+"\x00"+IP），
// 值为封禁截止毫秒时间戳（0 表示永久）。
func (b *BanList) Snapshot() map[string]int64 {
	m := *b.items.Load()
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Size 返回被封禁的 IP 数量（同一 IP 的全局与站点封禁只算一个）。
// 返回 int：按 IP 去重后的数量。
func (b *BanList) Size() int {
	m := *b.items.Load()
	seen := make(map[string]struct{}, len(m))
	for k := range m {
		ip := k
		if i := strings.IndexByte(k, 0); i >= 0 {
			ip = k[i+1:] // 站点封禁键：取 \x00 分隔符之后的 IP 部分
		}
		seen[ip] = struct{}{}
	}
	return len(seen)
}

// Cleanup 清理已过期的封禁条目，由后台协程定期调用。
// 参数 nowMS：当前毫秒时间戳。
// 返回 int：本次清理的条目数；没有过期条目时不替换快照。
// 永久封禁（截止时间为 0）不会被清理。
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

// ParseIP 解析客户端 IP 字符串。
// 参数 s：原始字符串，兼容 "1.2.3.4:5678"（带端口的 AddrPort）与裸 IP（含 IPv6）等形式。
// 返回 netip.Addr：解析后的地址（IPv4-mapped IPv6 已还原为 IPv4）；返回 bool：解析是否成功。
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

// Now 包级时间函数：业务代码统一经它取当前时间，测试可整体替换。
var Now = time.Now
