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

// State 是单个源的同步状态。
type State struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Scope    string `json:"scope"`
	Site     string `json:"site"`
	Count    int    `json:"count"`
	LastSync int64  `json:"lastSync"`
	Error    string `json:"error"`
}

// rangeSet 是只增不减的区间集合，支持 IPv4 区间与 IPv6 前缀。
type rangeSet struct {
	v4 []span
	v6 []netip.Prefix
}

type span struct {
	start uint32
	end   uint32
}

func (s *rangeSet) addPrefix(p netip.Prefix) {
	p = p.Masked()
	if !p.IsValid() {
		return
	}
	addr := p.Addr().Unmap()
	if addr.Is4() {
		start := binary.BigEndian.Uint32(addr.AsSlice())
		bits := p.Bits()
		var size uint32 = 32
		if bits < 32 {
			size = uint32(1) << (32 - bits)
		}
		s.v4 = append(s.v4, span{start: start, end: start + size - 1})
		return
	}
	s.v6 = append(s.v6, p)
}

func (s *rangeSet) addRange(lo, hi uint32) {
	if lo > hi {
		return
	}
	s.v4 = append(s.v4, span{start: lo, end: hi})
}

// compact 排序并合并重叠区间。
func (s *rangeSet) compact() {
	if len(s.v4) == 0 {
		return
	}
	sort.Slice(s.v4, func(i, j int) bool { return s.v4[i].start < s.v4[j].start })
	out := s.v4[:1]
	for _, sp := range s.v4[1:] {
		last := &out[len(out)-1]
		if sp.start <= last.end+1 {
			if sp.end > last.end {
				last.end = sp.end
			}
			continue
		}
		out = append(out, sp)
	}
	s.v4 = out
}

func (s *rangeSet) size() int { return len(s.v4) + len(s.v6) }

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
	for _, p := range s.v6 {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Manager 持有所有情报源的内存快照。
type Manager struct {
	mu     sync.RWMutex
	global    *rangeSet
	sites     map[string]*rangeSet
	states    map[uint]State
	perSource map[uint]*rangeSet
	client    *http.Client
}

// New 创建管理器。
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
func (m *Manager) Match(addr netip.Addr, site string) string {
	if m == nil {
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
func (m *Manager) GlobalCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.global.size()
}

// Stats 返回各源状态。
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

// SyncResult 单源同步结果。
type SyncResult struct {
	ID    uint
	Count int
	Error string
}

// Sync 拉取所有启用的源并重建快照。单个源失败不影响其他源（保留上次结果）。
func (m *Manager) Sync(ctx context.Context, sources []model.IntelSource) []SyncResult {
	results := make([]SyncResult, 0, len(sources))
	perSource := make(map[uint]*rangeSet, len(sources))

	for _, src := range sources {
		res := SyncResult{ID: src.ID}
		if src.Enabled != 1 {
			continue
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

// perSource 记录每个源上一次成功的数据，供失败兜底与重建使用。
func (m *Manager) snapshotOf(id uint) *rangeSet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.perSource == nil {
		return nil
	}
	return m.perSource[id]
}

func (m *Manager) setState(id uint, st State) {
	m.mu.Lock()
	m.states[id] = st
	m.mu.Unlock()
}

func mergeInto(dst, src *rangeSet) {
	if src == nil {
		return
	}
	dst.v4 = append(dst.v4, src.v4...)
	dst.v6 = append(dst.v6, src.v6...)
}

// fetch 拉取并解析一个列表。整份列表解析完成才返回，避免半份数据替换快照。
func (m *Manager) fetch(ctx context.Context, src model.IntelSource) (*rangeSet, error) {
	url := src.URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("只支持 http(s) 地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoWAF-Intel/1.0")
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
func ParseHeaders(raw string) map[string]string {
	out := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
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
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
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
func parseList(r io.Reader) (*rangeSet, error) {
	set := &rangeSet{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
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
		addToken(set, strings.Trim(line, "\"'"))
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

// addToken 把一个可能是 IP / CIDR / a-b 区间的字符串加进集合，非法值静默忽略。
func addToken(set *rangeSet, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if strings.Contains(line, "-") && !strings.Contains(line, "/") {
		parts := strings.SplitN(line, "-", 2)
		a, err1 := netip.ParseAddr(strings.TrimSpace(parts[0]))
		b, err2 := netip.ParseAddr(strings.TrimSpace(parts[1]))
		if err1 == nil && err2 == nil && a.Is4() && b.Is4() {
			set.addRange(binary.BigEndian.Uint32(a.AsSlice()), binary.BigEndian.Uint32(b.AsSlice()))
			return
		}
	}
	if strings.Contains(line, "/") {
		if p, err := netip.ParsePrefix(line); err == nil {
			set.addPrefix(p)
		}
		return
	}
	if addr, err := netip.ParseAddr(line); err == nil {
		bits := 32
		if !addr.Unmap().Is4() {
			bits = 128
		}
		set.addPrefix(netip.PrefixFrom(addr.Unmap(), bits))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
