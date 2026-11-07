// Package intel 实现威胁情报 IP 库的拉取与匹配。
//
// 思路：情报和正则规则彻底分家。规则处理 payload，情报处理「这个 IP 本身
// 就不该来」——已知的扫描器出口、僵尸网络、C2 段。匹配用排序区间二分，
// 单次查询几百纳秒，可以放在请求主链路上。
package intel

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"gowaf/internal/store/model"
)

// maxListBytes 单个情报列表的读取上限，防止误配大文件把内存撑爆。
const maxListBytes = 64 << 20

// State 是单个情报源的同步状态快照，用于面板展示与对外 API 输出。
type State struct {
	ID       uint   `json:"id"`       // 源在数据库中的唯一标识
	Name     string `json:"name"`     // 源名称
	URL      string `json:"url"`      // 订阅地址
	Scope    string `json:"scope"`    // 生效范围：global 全局，site 仅对指定站点生效
	Site     string `json:"site"`     // Scope 为 site 时绑定的站点标识
	Count    int    `json:"count"`    // 当前收录的区间/前缀条数
	LastSync int64  `json:"lastSync"` // 最近一次同步时间，Unix 毫秒时间戳
	Error    string `json:"error"`    // 最近一次同步的错误信息，成功时为空串
}

// rangeSet 是只增不减的 IP 区间集合，支持 IPv4 区间与 IPv6 前缀。
// addPrefix/addRange 只负责追加，追加完成后必须调用 compact 排序合并，
// 之后 contains 才能对 v4 做正确的二分查找。
type rangeSet struct {
	v4 []span         // 排序合并后的 IPv4 闭区间列表（单 IP 也展开为区间）
	v6 []netip.Prefix // IPv6 前缀列表，匹配时线性遍历
}

// span 表示一段 IPv4 地址的闭区间 [start, end]。
type span struct {
	start uint32 // 区间起始地址（IPv4 按大端序展开成的 32 位整数）
	end   uint32 // 区间结束地址（含端点）
}

// addPrefix 把一个 IP 前缀追加进集合。
// 参数 p：待加入的网段；会先 Masked 规范化去掉主机位，IPv4-mapped IPv6 地址还原为 IPv4 处理。
// 返回：无；非法前缀直接忽略。
func (s *rangeSet) addPrefix(p netip.Prefix) {
	p = p.Masked() // 规范化：去掉前缀里的主机位
	if !p.IsValid() {
		return
	}
	addr := p.Addr().Unmap() // IPv4-mapped IPv6（::ffff:a.b.c.d）还原成 IPv4 再判断
	if addr.Is4() {
		start := binary.BigEndian.Uint32(addr.AsSlice())
		bits := p.Bits()
		// 计算网段覆盖的地址数：2^(32-bits)。bits=32（单 IP）时不会进入分支，
		// size 保持初始值 32。
		var size uint32 = 32
		if bits < 32 {
			size = uint32(1) << (32 - bits)
		}
		s.v4 = append(s.v4, span{start: start, end: start + size - 1})
		return
	}
	// IPv6 前缀无法展开成区间，直接进前缀列表
	s.v6 = append(s.v6, p)
}

// addRange 把一段 IPv4 闭区间追加进集合。
// 参数 lo、hi：区间的起始与结束地址（32 位整数形式）。
// 返回：无；lo > hi 的非法区间直接忽略。
func (s *rangeSet) addRange(lo, hi uint32) {
	if lo > hi {
		return
	}
	s.v4 = append(s.v4, span{start: lo, end: hi})
}

// compact 对 v4 区间按起始地址排序，并把重叠或相邻（end+1 相接）的区间合并。
// 参数：无；返回：无（在原切片上原地合并，复用底层数组）。
// 只有 compact 之后 contains 的二分查找前提才成立。
func (s *rangeSet) compact() {
	if len(s.v4) == 0 {
		return
	}
	sort.Slice(s.v4, func(i, j int) bool { return s.v4[i].start < s.v4[j].start })
	// 以第一个区间为基准，在原切片头部原地合并
	out := s.v4[:1]
	for _, sp := range s.v4[1:] {
		last := &out[len(out)-1]
		// 当前区间起点落在上一区间内或与其紧邻，可并入
		if sp.start <= last.end+1 {
			if sp.end > last.end {
				last.end = sp.end // 仅向右扩展终点
			}
			continue
		}
		out = append(out, sp)
	}
	s.v4 = out
}

// size 返回集合中的条目总数（IPv4 区间数 + IPv6 前缀数）。
// 参数：无；返回：条目总数，供同步结果与面板展示使用。
func (s *rangeSet) size() int { return len(s.v4) + len(s.v6) }

// contains 判断地址是否落在集合内。
// 参数 addr：待检查的地址，IPv4-mapped IPv6 会先还原成 IPv4。
// 返回：命中任意 IPv4 区间或 IPv6 前缀时为 true；接收者为 nil 的空集合恒为 false。
func (s *rangeSet) contains(addr netip.Addr) bool {
	if s == nil {
		return false
	}
	addr = addr.Unmap()
	if addr.Is4() {
		if len(s.v4) == 0 {
			return false
		}
		ip := binary.BigEndian.Uint32(addr.AsSlice())
		// 标准二分：ip 小于当前区间起点则向左走，大于终点则向右走，否则命中。
		// 排序区间上的单次查询为 O(log n)，可放在请求主链路。
		lo, hi := 0, len(s.v4)-1
		for lo <= hi {
			mid := (lo + hi) / 2
			switch {
			case ip < s.v4[mid].start:
				hi = mid - 1
			case ip > s.v4[mid].end:
				lo = mid + 1
			default:
				return true
			}
		}
		return false
	}
	// IPv6 前缀数量少，线性遍历足够
	for _, p := range s.v6 {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Manager 持有所有情报源的内存快照，负责拉取、合并与匹配。
type Manager struct {
	mu        sync.RWMutex         // 保护下方全部字段；匹配走读锁，同步走写锁
	global    *rangeSet            // 全局情报区间集合（Scope 非 site 的源合并于此）
	sites     map[string]*rangeSet // 站点级情报：站点标识 -> 该站点的区间集合
	states    map[uint]State       // 各源的同步状态（含最近错误），按源 ID 索引
	perSource map[uint]*rangeSet   // 各源最近一次成功拉取的数据，供失败兜底
	client    *http.Client         // 拉取情报用的 HTTP 客户端
}

// New 创建一个空的情报管理器。
// 参数：无。
// 返回：初始化好全局集合、站点映射、状态映射与 HTTP 客户端（30 秒超时）的 Manager。
func New() *Manager {
	return &Manager{
		global: &rangeSet{},
		sites:  map[string]*rangeSet{},
		states: map[uint]State{},
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Match 判断 IP 是否命中情报。返回命中的源名（空串表示未命中）。
// 站点级情报优先于全局，便于按站点做差异化封禁。
//
// 特殊用途地址直接放行：公共情报源里偶尔会混进 127.0.0.0/8、10.0.0.0/8
// 这类 bogon（多来源聚合清单尤其是重灾区）。反代型 WAF 里真实客户端 IP 来自
// X-Forwarded-For，一旦上游没传这个头，所有请求的 clientIP 都会退化成 127.0.0.1，
// 这时情报若命中回环段就会把整站打成 403——这类自锁必须在匹配层彻底堵死。
//
// 参数 addr：待检查的客户端地址；参数 site：当前站点标识，空串表示不做站点级匹配。
// 返回：命中的情报类别名（"站点情报"/"全局情报"），未命中或管理器为 nil 时返回空串。
func (m *Manager) Match(addr netip.Addr, site string) string {
	if m == nil || isSpecialUse(addr) {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if site != "" {
		if rs, ok := m.sites[site]; ok && rs.contains(addr) {
			return "站点情报"
		}
	}
	if m.global.contains(addr) {
		return "全局情报"
	}
	return ""
}

// GlobalCount 返回全局区间数，供面板展示。
// 参数：无；返回：全局集合的条目数，管理器为 nil 或尚未同步时为 0。
func (m *Manager) GlobalCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.global.size()
}

// Stats 返回全部源的同步状态快照。
// 参数：无；返回：按源 ID 升序排列的状态副本，修改返回值不影响内部数据。
func (m *Manager) Stats() []State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]State, 0, len(m.states))
	for _, s := range m.states {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SyncResult 是单源同步的结果，随 Sync 返回给调用方落库或展示。
type SyncResult struct {
	ID    uint   // 源 ID
	Count int    // 本次成功拉取的条目数；失败时保留上次成功的数量
	Error string // 本次同步的错误信息，成功时为空串
}

// Sync 拉取所有启用的源并重建快照。单个源失败不影响其他源（保留上次结果）。
// 参数 ctx：用于取消/超时控制各源的 HTTP 拉取；参数 sources：数据库中的全部情报源配置。
// 返回：启用源的同步结果列表（禁用源不出现在结果中）。
func (m *Manager) Sync(ctx context.Context, sources []model.IntelSource) []SyncResult {
	results := make([]SyncResult, 0, len(sources))
	perSource := make(map[uint]*rangeSet, len(sources))

	for _, src := range sources {
		res := SyncResult{ID: src.ID}
		if src.Enabled != 1 {
			continue // 跳过停用的源：不拉取，也不记录状态
		}
		set, err := m.fetch(ctx, src)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			// 失败保留旧数据：网络抖动不应该让防护瞬间归零
			m.mu.RLock()
			if old, ok := m.states[src.ID]; ok {
				res.Count = old.Count
			}
			m.mu.RUnlock()
			m.setState(src.ID, State{
				ID: src.ID, Name: src.Name, URL: src.URL, Scope: src.Scope, Site: src.Site,
				Count: res.Count, LastSync: time.Now().UnixMilli(), Error: truncate(res.Error, 200),
			})
			continue
		}
		res.Count = set.size()
		perSource[src.ID] = set
		results = append(results, res)
		m.setState(src.ID, State{
			ID: src.ID, Name: src.Name, URL: src.URL, Scope: src.Scope, Site: src.Site,
			Count: res.Count, LastSync: time.Now().UnixMilli(),
		})
	}

	// 重建合并快照：站点维度的源单独一组，其余并入全局。
	global := &rangeSet{}
	sites := map[string]*rangeSet{}
	for _, src := range sources {
		if src.Enabled != 1 {
			continue
		}
		set, ok := perSource[src.ID]
		if !ok {
			// 本次失败的源：沿用上次数据，避免情报库被一次网络故障清空
			if old := m.snapshotOf(src.ID); old != nil {
				set = old
			} else {
				continue
			}
		}
		if src.Scope == "site" && src.Site != "" {
			target, ok := sites[src.Site]
			if !ok {
				target = &rangeSet{}
				sites[src.Site] = target
			}
			mergeInto(target, set)
			continue
		}
		mergeInto(global, set)
	}
	global.compact()
	for _, rs := range sites {
		rs.compact()
	}

	m.mu.Lock()
	m.global = global
	m.sites = sites
	m.perSource = perSource
	// 清掉已删除/已停用源的状态，避免面板上留下幽灵条目
	alive := make(map[uint]struct{}, len(sources))
	for _, src := range sources {
		alive[src.ID] = struct{}{}
	}
	for id := range m.states {
		if _, ok := alive[id]; !ok {
			delete(m.states, id)
		}
	}
	m.mu.Unlock()
	return results
}

// snapshotOf 返回指定源最近一次成功拉取的数据快照（perSource 字段的只读访问）。
// 参数 id：源 ID。
// 返回：该源的区间集合；从未成功同步过或 perSource 尚未初始化时返回 nil。
func (m *Manager) snapshotOf(id uint) *rangeSet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.perSource == nil {
		return nil
	}
	return m.perSource[id]
}

// setState 写入单个源的同步状态。
// 参数 id：源 ID；参数 st：要写入的状态快照。
// 返回：无。
func (m *Manager) setState(id uint, st State) {
	m.mu.Lock()
	m.states[id] = st
	m.mu.Unlock()
}

// mergeInto 把 src 的全部 IPv4 区间与 IPv6 前缀并入 dst。
// 参数 dst：目标集合；参数 src：来源集合，为 nil 时直接返回。
// 返回：无（dst 原地追加）。
func mergeInto(dst, src *rangeSet) {
	if src == nil {
		return
	}
	dst.v4 = append(dst.v4, src.v4...)
	dst.v6 = append(dst.v6, src.v6...)
}

// fetch 拉取并解析一个列表。整份列表解析完成才返回，避免半份数据替换快照。
// 参数 ctx：控制 HTTP 请求的生命周期（取消/超时）；参数 src：情报源配置（URL、Kind、自定义请求头）。
// 返回 (rangeSet, error)：解析出的区间集合；地址非法、请求失败或状态码非 200 时返回错误。
func (m *Manager) fetch(ctx context.Context, src model.IntelSource) (*rangeSet, error) {
	url := src.URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("只支持 http(s) 地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// 固定 UA，便于情报源侧识别本程序流量；超时由 New 里的 client 统一控制（30 秒）
	req.Header.Set("User-Agent", "GoWAF-Intel/1.0")
	// 叠加用户配置的自定义请求头（如 API Key、Accept）
	for k, v := range ParseHeaders(src.Headers) {
		req.Header.Set(k, v)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 把响应体前 160 字节带出来：API Key 失效、额度用尽、地址写错，
		// 服务端通常会在 body 里说清楚，比只报 "HTTP 401" 好排查得多。
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 160))
		msg := strings.TrimSpace(string(snippet))
		if msg == "" {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(msg, 150))
	}
	// 按 Kind 选择解析器；json 走结构无关的递归收集，其余按纯文本逐行解析。
	// LimitReader 限制读取量至 maxListBytes，防止异常大响应撑爆内存。
	if strings.EqualFold(src.Kind, "json") {
		return parseJSONList(io.LimitReader(resp.Body, maxListBytes))
	}
	return parseList(io.LimitReader(resp.Body, maxListBytes))
}

// ParseHeaders 解析订阅源的自定义请求头，两种写法都支持：
//
//	{"Authorization":"Bearer xxx","Key":"yyy"}
//	Authorization: Bearer xxx
//
// 行文本里以 # 开头的行视为注释；值中允许出现冒号（如时间戳、URL）。
// 参数 raw：用户在源配置里填写的原始字符串。
// 返回：请求头名到值的映射；输入为空或两种格式都解析不出时返回空映射（不报错）。
func ParseHeaders(raw string) map[string]string {
	out := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	// 以 { 开头优先按 JSON 对象解析；解析失败则回退到行文本格式
	if strings.HasPrefix(raw, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			for k, v := range m {
				if k = strings.TrimSpace(k); k != "" {
					out[k] = strings.TrimSpace(v)
				}
			}
			return out
		}
	}
	// 行文本格式：逐行解析 "Key: Value"
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // 跳过空行与注释行
		}
		// Cut 只按第一个冒号切分，值里的冒号原样保留
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// parseList 解析纯文本清单：每行一个 IP / CIDR / 起止区间，支持 # 与 ; 注释。
// 兼容 Spamhaus DROP、FireHOL、AbuseIPDB、自建清单等常见格式。
// 参数 r：清单内容读取器（调用方已用 LimitReader 限制到 maxListBytes）。
// 返回 (rangeSet, error)：合并排序后的区间集合；读取 I/O 出错时返回错误。
func parseList(r io.Reader) (*rangeSet, error) {
	set := &rangeSet{}
	sc := bufio.NewScanner(r)
	// 行缓冲初始 64KB、上限 1MB，容忍个别超长行而不至于被撑爆
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		// 截掉行内注释：# 或 ; 之后的内容全部丢弃
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 有些清单是 CSV：取第一列
		if i := strings.IndexAny(line, ",\t"); i > 0 {
			line = strings.TrimSpace(line[:i])
		}
		addToken(set, strings.Trim(line, "\"'")) // 去掉包裹 IP 的引号
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	set.compact()
	return set, nil
}

// parseJSONList 解析 JSON 形式的清单（AbuseIPDB、CrowdSec CTI 等）。
//
// 不绑定具体字段名：递归遍历整棵 JSON 树，把任何形如 IP/CIDR 的字符串收进来。
// 这类接口的字段五花八门（ipAddress / value / ip / blocked_ip …），
// 而且各家随时会加字段，逐个适配只会一直追不上；按值识别反而稳定。
//
// 参数 r：JSON 响应体读取器（内部用 LimitReader 限制到 maxListBytes）。
// 返回 (rangeSet, error)：收集到的区间集合；JSON 非法（含被截断）时返回错误。
func parseJSONList(r io.Reader) (*rangeSet, error) {
	var v any
	dec := json.NewDecoder(io.LimitReader(r, maxListBytes))
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	set := &rangeSet{}
	walkJSON(v, set)
	set.compact()
	return set, nil
}

// walkJSON 深度优先遍历 JSON 值，把所有字符串节点交给 addToken 识别。
// 参数 v：任意 JSON 值（对象/数组/字符串/数字/布尔）；参数 set：收集结果的区间集合。
// 返回：无；数字、布尔等非字符串节点直接忽略。
func walkJSON(v any, set *rangeSet) {
	switch t := v.(type) {
	case string:
		addToken(set, t)
	case []any:
		for _, item := range t {
			walkJSON(item, set)
		}
	case map[string]any:
		for _, item := range t {
			walkJSON(item, set)
		}
	}
}

// specialUsePrefixes 是 netip 没有内置判断、且一旦误判就会造成自锁的保留网段：
// 运营商级 NAT（100.64/10）、0.0.0.0/8、240/4 保留段。
// 回环/私网/链路本地/组播/ULA 由 netip 的方法覆盖。
//
// 文档示例段（192.0.2/24、198.51.100/24、203.0.113/24、2001:db8::/32）故意不收：
// 它们不可能出现在真实情报里，过滤它们没有安全收益，反而会误伤单测与演练数据。
var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

// isSpecialUse 判断地址是否属于「不可能是公网攻击者」的特殊用途网段。
// 非法地址也按特殊用途处理（宁可漏拦，不可自锁）。
// 参数 addr：待检查地址（IPv4-mapped IPv6 先还原成 IPv4）。
// 返回：回环/私网/链路本地/组播/保留段等命中时为 true，合法公网地址为 false。
func isSpecialUse(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsMulticast() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() {
		return true
	}
	for _, p := range specialUsePrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// addToken 把一个可能是 IP / CIDR / a-b 区间的字符串加进集合，非法值静默忽略。
// 参数 set：目标区间集合；参数 line：单个 token（调用前已去注释、CSV 列与引号）。
// 返回：无；识别为特殊用途网段或格式非法的 token 直接丢弃。
func addToken(set *rangeSet, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	// 含 - 且不含 /：按「起始-结束」区间处理，避免与 CIDR 记法混淆
	if strings.Contains(line, "-") && !strings.Contains(line, "/") {
		parts := strings.SplitN(line, "-", 2)
		a, err1 := netip.ParseAddr(strings.TrimSpace(parts[0]))
		b, err2 := netip.ParseAddr(strings.TrimSpace(parts[1]))
		// 区间记法仅支持 IPv4（两端都能解析且都是 v4 才收）
		if err1 == nil && err2 == nil && a.Is4() && b.Is4() {
			if isSpecialUse(a) && isSpecialUse(b) {
				return // 两端都属特殊用途才丢弃；横跨公网的区间保留
			}
			set.addRange(binary.BigEndian.Uint32(a.AsSlice()), binary.BigEndian.Uint32(b.AsSlice()))
			return
		}
	}
	if strings.Contains(line, "/") {
		if p, err := netip.ParsePrefix(line); err == nil {
			// 只丢弃「整段都是特殊用途」的聚合（如 127.0.0.0/8、10.0.0.0/8）；
			// 0.0.0.0/1 这类同时覆盖公网的大聚合按原样保留，避免误删真实情报。
			if p.Bits() >= 8 && isSpecialUse(p.Addr()) {
				return
			}
			set.addPrefix(p)
		}
		return
	}
	// 剩下的按单个 IP 解析，展开成 /32（IPv4）或 /128（IPv6）前缀
	if addr, err := netip.ParseAddr(line); err == nil {
		if isSpecialUse(addr) {
			return
		}
		bits := 32
		if !addr.Unmap().Is4() {
			bits = 128
		}
		set.addPrefix(netip.PrefixFrom(addr.Unmap(), bits))
	}
}

// truncate 按字节截断字符串到最多 n 字节，用于错误信息与状态字段的定长存储。
// 参数 s：原字符串；参数 n：最大字节数。
// 返回：不超过 n 字节的字符串（多字节字符可能被从中间切断）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
