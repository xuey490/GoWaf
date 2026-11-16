// Package cache 实现页面缓存、响应压缩与 WebP 图片转换。
//
// 设计要点：
//  1. 规则在 Reload 时编译成不可变快照，匹配路径无锁；
//  2. 缓存对象按 LRU + TTL 管理，容量与单对象上限均可配置，避免内存被大文件拖垮；
//  3. 只缓存可安全复用的匿名 GET/HEAD 响应，带 Set-Cookie、private/no-store 的响应一律不缓存；
//  4. gzip / brotli 使用纯 Go 实现，WebP 编码同样纯 Go，CGO_ENABLED=0 也能编译。
package cache

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"container/list"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // 注册 JPEG 解码器：用于把 JPEG 转成 WebP
	_ "image/png"  // 注册 PNG 解码器
	"io"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HugoSmits86/nativewebp"
	"github.com/andybalholm/brotli"
)

// 规则种类。guest 表示「仅匿名访客」可缓存，其余种类按路径匹配。
const (
	KindSuffix    = "suffix"    // 后缀匹配：/style.css
	KindExact     = "exact"     // 完全相等：/index.html
	KindPrefix    = "prefix"    // 前缀匹配：/static/
	KindRegex     = "regex"     // 正则匹配
	KindDirectory = "directory" // 目录匹配：/blog 同时命中 /blog 与 /blog/*
	KindGuest     = "guest"     // 匿名访客全站缓存（带 Cookie 的登录用户不缓存）
)

// 默认值。集中定义，避免散落在调用方形成魔法值。
const (
	DefaultMaxBytes       = 128 << 20 // 默认缓存总容量 128 MiB
	DefaultMaxObjectBytes = 2 << 20   // 单对象上限 2 MiB
	DefaultTTL            = 300       // 默认缓存 5 分钟
	DefaultCompressMin    = 1024      // 小于 1 KiB 的响应压缩收益太低
)

// RuleDef 是数据库缓存规则到引擎的输入。
type RuleDef struct {
	ID       uint
	Name     string
	Site     string
	Kind     string
	Pattern  string
	TTL      time.Duration
	Priority int
	Enabled  bool
}

// Rule 是编译后仍对外暴露的规则信息（不含正则等内部字段）。
type Rule struct {
	ID        uint          `json:"id"`
	Name      string        `json:"name"`
	Site      string        `json:"site"`
	Kind      string        `json:"kind"`
	Pattern   string        `json:"pattern"`
	TTL       time.Duration `json:"-"`
	TTLSecond int           `json:"ttlSeconds"`
	Priority  int           `json:"priority"`
	GuestOnly bool          `json:"guestOnly"`
}

// Settings 是页面加速的全局设置。
type Settings struct {
	Enabled          bool          // 缓存总开关
	MaxBytes         int64         // 缓存总容量（字节）
	MaxObjectBytes   int64         // 单对象最大缓存字节数
	DefaultTTL       time.Duration // 未配置规则时的默认 TTL
	Compress         bool          // 是否启用 gzip 压缩
	Brotli           bool          // 是否启用 brotli（优先级高于 gzip）
	WebP             bool          // 是否把 JPEG/PNG 转成 WebP
	CompressMinBytes int           // 触发压缩的最小字节数
}

// Entry 是一个缓存对象：状态码、响应头与响应体原件。
type Entry struct {
	Status   int
	Header   http.Header
	Body     []byte
	StoredAt time.Time
	TTL      time.Duration
	Size     int64

	// hits 是该对象被命中的次数，由数据面读取时累加，控制面列表直接展示。
	// 用原子量而非普通字段：多个连接会并发读同一个 Entry。
	hits atomic.Int64
}

// Stats 是缓存运行统计，供控制面展示。
type Stats struct {
	Enabled   bool  `json:"enabled"`
	Entries   int   `json:"entries"`
	Bytes     int64 `json:"bytes"`
	MaxBytes  int64 `json:"maxBytes"`
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Purges    int64 `json:"purges"`
}

// EntryInfo 是缓存对象的只读视图，供控制面「缓存列表」展示。
//
// 缓存键形如 site|host|path?query，这里拆成独立字段，避免前端再解析字符串；
// 时间统一用 Unix 毫秒，与事件表等其他接口保持一致。
type EntryInfo struct {
	Key           string `json:"key"`
	Site          string `json:"site"`
	Host          string `json:"host"`
	Path          string `json:"path"`
	Query         string `json:"query"`
	Filename      string `json:"filename"`
	ContentType   string `json:"contentType"`
	Status        int    `json:"status"`
	Size          int64  `json:"size"`
	Hits          int64  `json:"hits"`
	StoredAt      int64  `json:"storedAt"`
	ExpiresAt     int64  `json:"expiresAt"`
	TTLSeconds    int64  `json:"ttlSeconds"`
	AgeSeconds    int64  `json:"ageSeconds"`
	RemainSeconds int64  `json:"remainSeconds"`
}

// Manager 是页面加速的运行入口：规则快照原子替换，对象存储内部加锁。
type Manager struct {
	state atomic.Pointer[state]
	store *store

	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
	purges    atomic.Int64
}

// state 是不可变规则与设置快照。
type state struct {
	rules    []*compiledRule
	settings Settings
}

// compiledRule 是编译后的规则。
type compiledRule struct {
	rule   Rule
	re     *regexp.Regexp
	prefix string
}

// NewManager 创建页面缓存管理器；默认关闭，等待 Reload 注入配置。
func NewManager() *Manager {
	m := &Manager{}
	m.state.Store(&state{settings: defaultSettings()})
	m.store = newStore(DefaultMaxBytes)
	return m
}

// defaultSettings 返回关闭状态下的安全默认值。
func defaultSettings() Settings {
	return Settings{
		Enabled:          false,
		MaxBytes:         DefaultMaxBytes,
		MaxObjectBytes:   DefaultMaxObjectBytes,
		DefaultTTL:       DefaultTTL * time.Second,
		Compress:         true,
		Brotli:           true,
		WebP:             false,
		CompressMinBytes: DefaultCompressMin,
	}
}

// Reload 编译缓存规则并替换快照。
// 参数 defs：数据库读出的规则；参数 st：全局设置。
// 返回 error：正则非法时整体失败，旧快照保留。
func (m *Manager) Reload(defs []RuleDef, st Settings) error {
	if st.MaxBytes <= 0 {
		st.MaxBytes = DefaultMaxBytes
	}
	if st.MaxObjectBytes <= 0 || st.MaxObjectBytes > st.MaxBytes {
		st.MaxObjectBytes = DefaultMaxObjectBytes
		if st.MaxObjectBytes > st.MaxBytes {
			st.MaxObjectBytes = st.MaxBytes
		}
	}
	if st.DefaultTTL <= 0 {
		st.DefaultTTL = DefaultTTL * time.Second
	}
	if st.CompressMinBytes <= 0 {
		st.CompressMinBytes = DefaultCompressMin
	}

	next := &state{settings: st}
	for _, d := range defs {
		if !d.Enabled || strings.TrimSpace(d.Pattern) == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(d.Kind))
		switch kind {
		case KindSuffix, KindExact, KindPrefix, KindDirectory, KindGuest:
		case KindRegex:
			re, err := regexp.Compile(d.Pattern)
			if err != nil {
				return fmt.Errorf("缓存规则 %s 正则非法: %w", d.Name, err)
			}
			cr := &compiledRule{rule: Rule{
				ID: d.ID, Name: d.Name, Site: strings.TrimSpace(d.Site), Kind: kind,
				Pattern: d.Pattern, TTL: d.TTL, TTLSecond: int(d.TTL / time.Second), Priority: d.Priority,
			}, re: re}
			next.rules = append(next.rules, cr)
			continue
		case "":
			kind = KindSuffix
		default:
			return fmt.Errorf("缓存规则 %s 匹配类型非法: %s", d.Name, kind)
		}
		if d.TTL <= 0 {
			d.TTL = st.DefaultTTL
		}
		r := Rule{
			ID: d.ID, Name: d.Name, Site: strings.TrimSpace(d.Site), Kind: kind,
			Pattern: d.Pattern, TTL: d.TTL, TTLSecond: int(d.TTL / time.Second),
			Priority: d.Priority, GuestOnly: kind == KindGuest,
		}
		next.rules = append(next.rules, &compiledRule{rule: r, prefix: strings.TrimSuffix(d.Pattern, "*")})
	}
	sort.SliceStable(next.rules, func(i, j int) bool {
		if next.rules[i].rule.Priority == next.rules[j].rule.Priority {
			return next.rules[i].rule.ID < next.rules[j].rule.ID
		}
		return next.rules[i].rule.Priority < next.rules[j].rule.Priority
	})
	m.state.Store(next)
	m.store.setMax(st.MaxBytes)
	return nil
}

// UpdateSettings 只更新全局设置，保留现有规则（用于开关切换等场景）。
// 参数 st：新设置。返回 error：内部压缩规则重建失败时返回错误。
func (m *Manager) UpdateSettings(st Settings) error {
	cur := m.state.Load()
	var defs []RuleDef
	if cur != nil {
		for _, r := range cur.rules {
			defs = append(defs, RuleDef{
				ID: r.rule.ID, Name: r.rule.Name, Site: r.rule.Site, Kind: r.rule.Kind,
				Pattern: r.rule.Pattern, TTL: r.rule.TTL, Priority: r.rule.Priority, Enabled: true,
			})
		}
	}
	return m.Reload(defs, st)
}

// Settings 返回当前设置快照。
func (m *Manager) Settings() Settings {
	if st := m.state.Load(); st != nil {
		return st.settings
	}
	return defaultSettings()
}

// RuleCount 返回已编译规则数量。
func (m *Manager) RuleCount() int {
	if st := m.state.Load(); st != nil {
		return len(st.rules)
	}
	return 0
}

// Enabled 判断缓存总开关是否开启。
func (m *Manager) Enabled() bool {
	st := m.state.Load()
	return st != nil && st.settings.Enabled
}

// Match 按站点与路径匹配缓存规则。
// 参数 site：站点名；path：请求路径；guest：是否匿名访客（无 Cookie/Authorization）。
// 返回 (Rule, bool)：命中返回规则副本；未命中或总开关关闭时返回 false。
func (m *Manager) Match(site, path string, guest bool) (Rule, bool) {
	st := m.state.Load()
	if st == nil || !st.settings.Enabled {
		return Rule{}, false
	}
	for _, cr := range st.rules {
		r := cr.rule
		if r.Site != "" && !strings.EqualFold(r.Site, site) {
			continue
		}
		if r.Kind == KindGuest && !guest {
			continue
		}
		if cr.match(path, guest) {
			if r.TTL <= 0 {
				r.TTL = st.settings.DefaultTTL
				r.TTLSecond = int(r.TTL / time.Second)
			}
			return r, true
		}
	}
	return Rule{}, false
}

// match 判断单个规则是否命中路径。
func (cr *compiledRule) match(path string, guest bool) bool {
	switch cr.rule.Kind {
	case KindSuffix:
		return strings.HasSuffix(path, cr.rule.Pattern)
	case KindExact:
		return path == cr.rule.Pattern
	case KindPrefix:
		return strings.HasPrefix(path, cr.rule.Pattern)
	case KindRegex:
		return cr.re != nil && cr.re.MatchString(path)
	case KindDirectory:
		p := cr.rule.Pattern
		if p == "" {
			return false
		}
		if path == p {
			return true
		}
		if !strings.HasSuffix(p, "/") {
			p += "/"
		}
		return strings.HasPrefix(path, p)
	case KindGuest:
		if !guest {
			return false
		}
		p := cr.prefix
		return p == "" || p == "*" || strings.HasPrefix(path, p)
	}
	return false
}

// Key 生成缓存键：站点 + Host + 路径 + Query，避免不同域名/参数串味。
func Key(site, host, path, query string) string {
	return site + "|" + strings.ToLower(host) + "|" + path + "?" + query
}

// Get 读取缓存对象。
// 参数 key：缓存键；参数 now：当前时间（便于测试）。
// 返回 (*Entry, bool)：命中且未过期返回 true；过期对象会被移除并计为未命中。
func (m *Manager) Get(key string, now time.Time) (*Entry, bool) {
	e, ok := m.store.get(key, now)
	if ok {
		m.hits.Add(1)
		return e, true
	}
	if m.Enabled() {
		m.misses.Add(1)
	}
	return nil, false
}

// Put 写入缓存对象；超过单对象上限或容量为 0 时直接丢弃。
// 参数 key：缓存键；参数 e：响应对象；参数 ttl：有效期。
func (m *Manager) Put(key string, e *Entry, ttl time.Duration) {
	if e == nil || len(e.Body) == 0 {
		return
	}
	st := m.state.Load()
	if st == nil || !st.settings.Enabled {
		return
	}
	if int64(len(e.Body)) > st.settings.MaxObjectBytes {
		return
	}
	if ttl <= 0 {
		ttl = st.settings.DefaultTTL
	}
	e.StoredAt = time.Now()
	e.TTL = ttl
	e.Size = int64(len(e.Body))
	if n := m.store.put(key, e); n > 0 {
		m.evictions.Add(int64(n))
	}
}

// Purge 清空缓存；site 非空时只清该站点，prefix 非空时再按路径前缀过滤。
// 参数 site：站点名，空串表示全部；prefix：缓存键中的路径前缀，空串表示不过滤。
// 返回 int：删除的对象数量。
func (m *Manager) Purge(site, prefix string) int {
	n := m.store.purge(site, prefix)
	if n > 0 {
		m.purges.Add(int64(n))
	}
	return n
}

// Entries 返回当前全部有效缓存对象的只读快照（已剔除过期但尚未清理的条目）。
// 参数 now：当前时间，便于测试。返回按写入时间倒序排列。
func (m *Manager) Entries(now time.Time) []EntryInfo {
	if m == nil || m.store == nil {
		return nil
	}
	return m.store.snapshot(now)
}

// Cleanup 删除过期缓存对象，由后台维护循环调用。
// 参数 now：当前时间。
func (m *Manager) Cleanup(now time.Time) {
	if m.store != nil {
		m.store.cleanup(now)
	}
}

// Stats 返回当前统计快照。
func (m *Manager) Stats() Stats {
	st := m.state.Load()
	out := Stats{
		Hits: m.hits.Load(), Misses: m.misses.Load(),
		Evictions: m.evictions.Load(), Purges: m.purges.Load(),
	}
	if st != nil {
		out.Enabled = st.settings.Enabled
		out.MaxBytes = st.settings.MaxBytes
	}
	out.Entries, out.Bytes = m.store.size()
	return out
}

/* ====================================================== 压缩与 WebP */

// Compressible 判断内容类型是否适合文本压缩。
func Compressible(contentType string) bool {
	ct := strings.ToLower(contentType)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/javascript", ct == "application/x-javascript",
		ct == "application/json", ct == "application/xml", ct == "application/xhtml+xml",
		ct == "image/svg+xml", ct == "application/rss+xml", ct == "application/atom+xml":
		return true
	}
	return false
}

// AcceptsEncoding 判断客户端 Accept-Encoding 是否接受指定编码。
// 参数 header：Accept-Encoding 头；参数 enc：gzip/br。
// 返回 bool：接受返回 true；q=0 视为拒绝。
func AcceptsEncoding(header, enc string) bool {
	enc = strings.ToLower(enc)
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		name := part
		q := 1.0
		if i := strings.IndexByte(part, ';'); i >= 0 {
			name = strings.TrimSpace(part[:i])
			for _, attr := range strings.Split(part[i+1:], ";") {
				attr = strings.TrimSpace(attr)
				if strings.HasPrefix(attr, "q=") {
					if v, err := strconv.ParseFloat(strings.TrimSpace(attr[2:]), 64); err == nil {
						q = v
					}
				}
			}
		}
		if (name == enc || name == "*") && q > 0 {
			return true
		}
	}
	return false
}

// Decodable 判断上游 Content-Encoding 是否支持解码。
// 参数 encoding：响应头 Content-Encoding 的值。
// 返回 bool：gzip/br/deflate 返回 true，其余编码由调用方透传。
func Decodable(encoding string) bool {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "gzip", "x-gzip", "br", "deflate":
		return true
	}
	return false
}

// Decode 按 Content-Encoding 把上游响应体解回原始字节。
// 参数 body：已压缩的响应体；参数 encoding：Content-Encoding 值。
// 返回 (data, ok)：ok=false 时调用方必须保持原样透传。
func Decode(body []byte, encoding string) ([]byte, bool) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return body, true
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, false
		}
		defer func() { _ = zr.Close() }()
		out, err := io.ReadAll(zr)
		if err != nil {
			return nil, false
		}
		return out, true
	case "br":
		out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(body)))
		if err != nil {
			return nil, false
		}
		return out, true
	case "deflate":
		fr := flate.NewReader(bytes.NewReader(body))
		defer func() { _ = fr.Close() }()
		out, err := io.ReadAll(fr)
		if err != nil {
			return nil, false
		}
		return out, true
	}
	return nil, false
}

// Compress 按客户端能力选择 brotli 或 gzip 压缩。
// 参数 body：原始响应体；参数 accept：Accept-Encoding 头；参数 preferBrotli：是否允许 br。
// 返回 (data, encoding, ok)：ok=false 表示未压缩，调用方保持原样输出。
func Compress(body []byte, accept string, preferBrotli bool) ([]byte, string, bool) {
	if preferBrotli && AcceptsEncoding(accept, "br") {
		var buf bytes.Buffer
		w := brotli.NewWriterLevel(&buf, brotli.DefaultCompression)
		if _, err := w.Write(body); err != nil {
			return body, "", false
		}
		if err := w.Close(); err != nil {
			return body, "", false
		}
		if buf.Len() >= len(body) {
			return body, "", false
		}
		return buf.Bytes(), "br", true
	}
	if AcceptsEncoding(accept, "gzip") {
		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err != nil {
			return body, "", false
		}
		if _, err := w.Write(body); err != nil {
			_ = w.Close()
			return body, "", false
		}
		if err := w.Close(); err != nil {
			return body, "", false
		}
		if buf.Len() >= len(body) {
			return body, "", false
		}
		return buf.Bytes(), "gzip", true
	}
	return body, "", false
}

// WebPConvertible 判断内容类型是否为可转 WebP 的图片。
func WebPConvertible(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct == "image/jpeg" || ct == "image/jpg" || ct == "image/png"
}

// ConvertWebP 把 JPEG/PNG 图片转成 WebP。
// 参数 data：原始图片字节；返回 (webp, ok)：解码或编码失败时 ok=false，调用方保留原图。
func ConvertWebP(data []byte) ([]byte, bool) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	var buf bytes.Buffer
	opts := &nativewebp.Options{CompressionLevel: nativewebp.DefaultCompression}
	if err := nativewebp.Encode(&buf, img, opts); err != nil {
		return nil, false
	}
	if buf.Len() == 0 || buf.Len() >= len(data) {
		return nil, false
	}
	return buf.Bytes(), true
}

// MediaType 提取 Content-Type 的媒体类型部分（去掉 charset 等参数）。
func MediaType(contentType string) string {
	if mt, _, err := mime.ParseMediaType(contentType); err == nil {
		return mt
	}
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		return strings.TrimSpace(contentType[:i])
	}
	return strings.TrimSpace(contentType)
}

/* ====================================================== LRU 存储 */

// store 是带容量上限的 LRU + TTL 存储。
type store struct {
	mu    sync.Mutex
	max   int64
	bytes int64
	ll    *list.List
	items map[string]*list.Element
}

// storeItem 是链表节点承载的键值对。
type storeItem struct {
	key string
	ent *Entry
}

// newStore 创建指定容量的存储。
func newStore(max int64) *store {
	return &store{max: max, ll: list.New(), items: make(map[string]*list.Element)}
}

// setMax 调整容量上限，必要时淘汰旧对象。
func (s *store) setMax(max int64) {
	if max <= 0 {
		max = DefaultMaxBytes
	}
	s.mu.Lock()
	s.max = max
	s.evictLocked()
	s.mu.Unlock()
}

// get 读取对象并在过期时删除。
func (s *store) get(key string, now time.Time) (*Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[key]
	if !ok {
		return nil, false
	}
	it := el.Value.(*storeItem)
	if it.ent.TTL > 0 && now.Sub(it.ent.StoredAt) > it.ent.TTL {
		s.removeLocked(el)
		return nil, false
	}
	s.ll.MoveToFront(el)
	it.ent.hits.Add(1)
	return it.ent, true
}

// put 写入对象，返回本次淘汰的对象数量。
func (s *store) put(key string, ent *Entry) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.max <= 0 {
		return 0
	}
	if el, ok := s.items[key]; ok {
		old := el.Value.(*storeItem)
		s.bytes -= old.ent.Size
		old.ent = ent
		s.bytes += ent.Size
		s.ll.MoveToFront(el)
		return s.evictLocked()
	}
	el := s.ll.PushFront(&storeItem{key: key, ent: ent})
	s.items[key] = el
	s.bytes += ent.Size
	return s.evictLocked()
}

// purge 删除匹配的缓存对象，返回删除数量。
func (s *store) purge(site, prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for key, el := range s.items {
		if site != "" && !strings.HasPrefix(key, site+"|") {
			continue
		}
		if prefix != "" {
			// key 形如 site|host|path?query，取第三个字段做前缀比较
			parts := strings.SplitN(key, "|", 3)
			if len(parts) < 3 || !strings.HasPrefix(parts[2], prefix) {
				continue
			}
		}
		s.removeLocked(el)
		n++
	}
	return n
}

// cleanup 删除全部过期对象。
func (s *store) cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, el := range s.items {
		it := el.Value.(*storeItem)
		if it.ent.TTL > 0 && now.Sub(it.ent.StoredAt) > it.ent.TTL {
			s.removeLocked(el)
		}
	}
}

// size 返回对象数量与占用字节数。
func (s *store) size() (int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items), s.bytes
}

// snapshot 生成缓存对象列表，过期条目直接跳过（后台 Cleanup 可能还没跑到）。
func (s *store) snapshot(now time.Time) []EntryInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]EntryInfo, 0, len(s.items))
	for key, el := range s.items {
		e := el.Value.(*storeItem).ent
		if e == nil {
			continue
		}
		if e.TTL > 0 && now.Sub(e.StoredAt) > e.TTL {
			continue
		}
		site, host, path, query := splitKey(key)
		age := int64(now.Sub(e.StoredAt) / time.Second)
		if age < 0 {
			age = 0
		}
		info := EntryInfo{
			Key: key, Site: site, Host: host, Path: path, Query: query,
			Filename:    baseName(path),
			ContentType: e.Header.Get("Content-Type"),
			Status:      e.Status, Size: e.Size, Hits: e.hits.Load(),
			StoredAt: e.StoredAt.UnixMilli(), AgeSeconds: age,
			TTLSeconds: int64(e.TTL / time.Second), RemainSeconds: -1,
		}
		if e.TTL > 0 {
			expire := e.StoredAt.Add(e.TTL)
			info.ExpiresAt = expire.UnixMilli()
			remain := int64(expire.Sub(now) / time.Second)
			if remain < 0 {
				remain = 0
			}
			info.RemainSeconds = remain
		}
		out = append(out, info)
	}
	// 最近写入的排前面：控制面排查「刚缓存的为什么没命中」时最常看新对象。
	sort.Slice(out, func(i, j int) bool {
		if out[i].StoredAt == out[j].StoredAt {
			return out[i].Path < out[j].Path
		}
		return out[i].StoredAt > out[j].StoredAt
	})
	return out
}

// splitKey 把缓存键 site|host|path?query 拆回独立字段。
func splitKey(key string) (site, host, path, query string) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) < 3 {
		return "", "", key, ""
	}
	site, host, rest := parts[0], parts[1], parts[2]
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		return site, host, rest[:i], rest[i+1:]
	}
	return site, host, rest, ""
}

// baseName 取路径最后一段作为展示用文件名；目录型路径原样返回。
func baseName(p string) string {
	if p == "" {
		return "/"
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}

// evictLocked 从尾部淘汰对象直到容量回到上限内。
func (s *store) evictLocked() int {
	n := 0
	for s.bytes > s.max && s.ll.Len() > 0 {
		s.removeLocked(s.ll.Back())
		n++
	}
	return n
}

// removeLocked 删除链表节点并同步计数。
func (s *store) removeLocked(el *list.Element) {
	if el == nil {
		return
	}
	it := el.Value.(*storeItem)
	delete(s.items, it.key)
	s.bytes -= it.ent.Size
	s.ll.Remove(el)
}

// MarshalJSON 保证 Entry 在控制面调试接口里可读（Body 用长度替代，避免刷屏）。
func (e *Entry) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	return json.Marshal(map[string]any{
		"status": e.Status, "size": e.Size, "storedAt": e.StoredAt.UnixMilli(),
		"ttlSeconds": int(e.TTL / time.Second), "hits": e.hits.Load(),
	})
}

// CopyBody 返回响应体副本，避免调用方修改缓存对象。
func (e *Entry) CopyBody() []byte {
	if e == nil {
		return nil
	}
	out := make([]byte, len(e.Body))
	copy(out, e.Body)
	return out
}

// WriteTo 把缓存对象写回客户端（不含压缩，由调用方统一处理）。
func (e *Entry) WriteTo(w http.ResponseWriter) {
	if e == nil {
		return
	}
	for k, vs := range e.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(e.Status)
	_, _ = w.Write(e.Body)
}

// DecodeSettings 把设置表里的字符串键解析成 Settings。
// 参数 get：按键取值函数；参数 def：缺省值。
// 返回 Settings：缺失或非法的键回落到 def 对应字段。
func DecodeSettings(get func(string) string, def Settings) Settings {
	out := def
	parseBool := func(key string, dst *bool) {
		if v := strings.TrimSpace(get(key)); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				*dst = b
			}
		}
	}
	parseInt := func(key string, dst *int) {
		if v := strings.TrimSpace(get(key)); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				*dst = n
			}
		}
	}
	parseInt64 := func(key string, dst *int64) {
		if v := strings.TrimSpace(get(key)); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
				*dst = n
			}
		}
	}
	parseBool("cache.enabled", &out.Enabled)
	parseInt64("cache.max_bytes", &out.MaxBytes)
	parseInt64("cache.max_object_bytes", &out.MaxObjectBytes)
	parseBool("cache.compress", &out.Compress)
	parseBool("cache.brotli", &out.Brotli)
	parseBool("cache.webp", &out.WebP)
	parseInt("cache.compress_min_bytes", &out.CompressMinBytes)
	if v := strings.TrimSpace(get("cache.default_ttl")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			out.DefaultTTL = time.Duration(n) * time.Second
		}
	}
	return out
}

// DefaultSettings 返回可在控制面展示/落库的默认设置。
func DefaultSettings() Settings { return defaultSettings() }
