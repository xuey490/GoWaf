// Package policy 实现精准访问控制策略引擎。
//
// 设计目标：
//  1. 条件树（AND / OR / 嵌套分组）在 Reload 时编译成不可变快照，热路径无锁、无 JSON 解析；
//  2. 字段覆盖 IP、Geo、Host、Method、Path、Query、Header、Cookie、参数、UA、Referer、Body；
//  3. 动作支持 allow/block/challenge/log/ban，第一条命中的策略生效；
//  4. 编译失败整体拒绝，旧快照保留，避免半套策略把站点放空。
package policy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// Action 是策略命中后的处置动作。
type Action string

const (
	ActionAllow     Action = "allow"     // 放行：跳过后续 WAF 检测
	ActionBlock     Action = "block"     // 拦截：返回 403
	ActionChallenge Action = "challenge" // 人机校验
	ActionLog       Action = "log"       // 只记录，继续后续检测
	ActionBan       Action = "ban"       // 立即封禁来源 IP
)

// Condition 是单个匹配条件。
type Condition struct {
	Field      string `json:"field"`                // 字段名，如 path / header.X-Foo / arg.q
	Op         string `json:"op"`                   // 操作符
	Value      string `json:"value"`                // 比较值；exists 时为 true/false
	IgnoreCase bool   `json:"ignoreCase,omitempty"` // 字符串比较是否忽略大小写
}

// Group 是条件树节点：同一层 items 与 groups 按 Logic 组合。
type Group struct {
	Logic  string      `json:"logic"`            // and / or
	Items  []Condition `json:"items,omitempty"`  // 叶子条件
	Groups []Group     `json:"groups,omitempty"` // 嵌套分组
}

// Definition 是数据库策略到引擎的输入。
type Definition struct {
	ID         uint
	Name       string
	Site       string
	Priority   int
	Enabled    bool
	Action     Action
	Conditions Group
}

// Request 是策略求值上下文，由 edge 层按请求构造一次。
type Request struct {
	ClientIP    string
	Country     string
	Region      string
	Host        string
	Method      string
	Path        string
	Query       string
	UA          string
	Referer     string
	ContentType string
	Body        string
	Header      http.Header
	Cookie      map[string]string
	Args        map[string]string
}

// NewRequest 从 HTTP 请求构造策略上下文。
// 参数 r：客户端请求；body：已读取的请求体；ip/country/region：已解析的客户端 IP 与归属地。
// 返回 *Request：Header 使用原 http.Header（Get 大小写不敏感），Cookie/Args 已展开。
func NewRequest(r *http.Request, body []byte, ip, country, region string) *Request {
	req := &Request{
		ClientIP:    ip,
		Country:     country,
		Region:      region,
		Host:        r.Host,
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.RawQuery,
		UA:          r.UserAgent(),
		Referer:     r.Referer(),
		ContentType: r.Header.Get("Content-Type"),
		Body:        string(body),
		Header:      r.Header,
		Cookie:      make(map[string]string, 8),
		Args:        make(map[string]string, 16),
	}
	for _, ck := range r.Cookies() {
		req.Cookie[ck.Name] = ck.Value
	}
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			req.Args[k] = vs[0]
		}
	}
	if strings.HasPrefix(strings.ToLower(req.ContentType), "application/x-www-form-urlencoded") && len(body) > 0 {
		if vals, err := url.ParseQuery(string(body)); err == nil {
			for k, vs := range vals {
				if _, exists := req.Args[k]; !exists && len(vs) > 0 {
					req.Args[k] = vs[0]
				}
			}
		}
	}
	return req
}

// Match 是一次策略命中的结果。
type Match struct {
	PolicyID uint   `json:"policyId"`
	Name     string `json:"name"`
	Action   Action `json:"action"`
	Reason   string `json:"reason"`
}

// Set 是编译后的不可变策略快照，可并发读取。
type Set struct {
	policies []*compiledPolicy
	count    int
}

// HitStat 是单条策略的命中统计。
type HitStat struct {
	ID      uint  `json:"id"`
	Hits    int64 `json:"hits"`
	LastHit int64 `json:"lastHit"` // Unix 毫秒
}

// compiledPolicy 是编译后的策略。
type compiledPolicy struct {
	id       uint
	name     string
	site     string
	priority int
	action   Action
	root     *compiledGroup
	// needsBody 表示条件树引用了 body 字段。这类策略必须在读取请求体之后才能求值，
	// 因此无法在「缓存查找之前」提前决策。
	needsBody bool
	hits      atomic.Int64 // 累计命中次数，由数据面写入、后台任务定期刷库
	lastHit   atomic.Int64 // 最近命中时间（Unix 毫秒）
}

// compiledGroup 是编译后的条件树节点。
type compiledGroup struct {
	logic  string
	items  []*compiledCondition
	groups []*compiledGroup
}

// compiledCondition 是编译后的叶子条件。
type compiledCondition struct {
	field      string
	op         string
	value      string
	ignoreCase bool
	re         *regexp.Regexp
	prefix     *netip.Prefix
	num        float64
	in         []string
}

// Compile 把策略定义编译成不可变快照。
// 参数 defs：数据库读出的策略；Enabled=false 的定义会被跳过。
// 返回 (*Set, error)：任一策略条件非法时整体失败，调用方应保留旧快照。
func Compile(defs []Definition) (*Set, error) {
	s := &Set{}
	for _, d := range defs {
		if !d.Enabled {
			continue
		}
		action := Action(strings.ToLower(strings.TrimSpace(string(d.Action))))
		if action == "" {
			action = ActionBlock
		}
		switch action {
		case ActionAllow, ActionBlock, ActionChallenge, ActionLog, ActionBan:
		default:
			return nil, fmt.Errorf("策略 %s 动作非法: %s", d.Name, action)
		}
		if d.Name == "" {
			return nil, fmt.Errorf("策略 ID %d 名称为空", d.ID)
		}
		root, err := compileGroup(d.Conditions, 0)
		if err != nil {
			return nil, fmt.Errorf("策略 %s: %w", d.Name, err)
		}
		if root.empty() {
			return nil, fmt.Errorf("策略 %s 条件为空（会匹配所有请求）", d.Name)
		}
		p := &compiledPolicy{
			id: d.ID, name: d.Name, site: strings.TrimSpace(d.Site),
			priority: d.Priority, action: action, root: root,
			needsBody: root.hasBodyDependency(),
		}
		s.policies = append(s.policies, p)
	}
	sort.SliceStable(s.policies, func(i, j int) bool {
		if s.policies[i].priority == s.policies[j].priority {
			return s.policies[i].id < s.policies[j].id
		}
		return s.policies[i].priority < s.policies[j].priority
	})
	s.count = len(s.policies)
	return s, nil
}

// Count 返回启用的策略数量。
func (s *Set) Count() int {
	if s == nil {
		return 0
	}
	return s.count
}

// RecordHit 记录一次策略命中，供控制面命中统计使用。
// 参数 id：策略 ID；参数 nowMS：当前毫秒时间戳。
func (s *Set) RecordHit(id uint, nowMS int64) {
	if s == nil {
		return
	}
	for _, p := range s.policies {
		if p.id == id {
			p.hits.Add(1)
			p.lastHit.Store(nowMS)
			return
		}
	}
}

// HitStats 返回全部策略的命中统计快照。
func (s *Set) HitStats() []HitStat {
	if s == nil {
		return nil
	}
	out := make([]HitStat, 0, len(s.policies))
	for _, p := range s.policies {
		out = append(out, HitStat{ID: p.id, Hits: p.hits.Load(), LastHit: p.lastHit.Load()})
	}
	return out
}

// ResetHits 将命中计数归零（刷库成功后调用，避免重复累加）。
// 参数 id：策略 ID；参数 delta：本次已落库的增量。
func (s *Set) ResetHits(id uint, delta int64) {
	if s == nil || delta <= 0 {
		return
	}
	for _, p := range s.policies {
		if p.id == id {
			p.hits.Add(-delta)
			return
		}
	}
}

// Evaluate 按优先级顺序求值，返回第一条命中的策略。
// 参数 req：策略上下文；site：当前站点名（空策略 site 对所有站点生效）。
// 返回 (*Match, bool)：命中返回策略结果；未命中返回 nil,false。
func (s *Set) Evaluate(req *Request, site string) (*Match, bool) {
	if s == nil || req == nil {
		return nil, false
	}
	for _, p := range s.policies {
		if p.site != "" && !strings.EqualFold(p.site, site) {
			continue
		}
		if p.root.match(req) {
			return &Match{
				PolicyID: p.id, Name: p.name, Action: p.action,
				Reason: "精准访问控制：" + p.name,
			}, true
		}
	}
	return nil, false
}

// EvaluateDenyEarly 在读取请求体之前做一次「可提前决策」的求值。
//
// 存在的理由：数据面的顺序是「缓存查找 → 读 body → 完整策略求值」，
// 而缓存命中会直接返回响应。若不提前判一次，命中缓存的静态资源就会
// 绕过 block / ban / challenge 这类拒绝动作（实测可复现：已缓存的 js
// 对境外 IP 仍返回 200）。
//
// 语义上严格保持「优先级第一条命中的策略生效」：按优先级遍历，
// 只有「第一条命中的策略」同时满足（1）不依赖 body、（2）动作是拒绝类，
// 才返回命中。若第一条命中依赖 body，或动作是 allow / log，一律返回
// 未命中，把决策原样交回第 8 步的完整求值——宁可晚判，也不改变结论。
//
// 参数 req：策略上下文，body 字段应为空；参数 site：当前站点名。
// 返回 (*Match, bool)：命中的策略与是否命中。
func (s *Set) EvaluateDenyEarly(req *Request, site string) (*Match, bool) {
	if s == nil || req == nil {
		return nil, false
	}
	for _, p := range s.policies {
		if p.site != "" && !strings.EqualFold(p.site, site) {
			continue
		}
		if !p.root.match(req) {
			continue
		}
		// 第一条命中的策略即最终结论：无法提前判定就交回完整求值。
		if p.needsBody {
			return nil, false
		}
		switch p.action {
		case ActionBlock, ActionBan, ActionChallenge:
			return &Match{
				PolicyID: p.id, Name: p.name, Action: p.action,
				Reason: "精准访问控制：" + p.name,
			}, true
		}
		return nil, false
	}
	return nil, false
}

// DecodeGroup 解析数据库中的条件树 JSON。
// 参数 raw：JSON 文本；空串返回空 AND 组。
func DecodeGroup(raw string) (Group, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Group{Logic: "and"}, nil
	}
	var g Group
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return Group{}, err
	}
	if g.Logic == "" {
		g.Logic = "and"
	}
	return g, nil
}

// compileGroup 递归编译条件树，限制嵌套深度与条件总数，防止策略把热路径拖垮。
func compileGroup(g Group, depth int) (*compiledGroup, error) {
	if depth > 4 {
		return nil, fmt.Errorf("条件嵌套超过 4 层")
	}
	logic := strings.ToLower(strings.TrimSpace(g.Logic))
	if logic == "" {
		logic = "and"
	}
	if logic != "and" && logic != "or" {
		return nil, fmt.Errorf("逻辑只能是 and 或 or")
	}
	out := &compiledGroup{logic: logic}
	total := 0
	for _, item := range g.Items {
		c, err := compileCondition(item)
		if err != nil {
			return nil, err
		}
		out.items = append(out.items, c)
		total++
	}
	for _, sub := range g.Groups {
		cg, err := compileGroup(sub, depth+1)
		if err != nil {
			return nil, err
		}
		if cg.empty() {
			continue
		}
		out.groups = append(out.groups, cg)
		total++
	}
	if total > 64 {
		return nil, fmt.Errorf("单条策略条件超过 64 个")
	}
	return out, nil
}

// compileCondition 编译叶子条件：校验字段、操作符与比较值。
func compileCondition(c Condition) (*compiledCondition, error) {
	field := strings.ToLower(strings.TrimSpace(c.Field))
	op := strings.ToLower(strings.TrimSpace(c.Op))
	if field == "" {
		return nil, fmt.Errorf("条件字段为空")
	}
	if !validField(field) {
		return nil, fmt.Errorf("不支持的字段: %s", field)
	}
	out := &compiledCondition{field: field, op: op, value: c.Value, ignoreCase: c.IgnoreCase}
	switch op {
	case "eq", "ne", "contains", "prefix", "suffix":
		if c.Value == "" {
			return nil, fmt.Errorf("字段 %s 的 %s 比较值不能为空", field, op)
		}
		if c.IgnoreCase {
			out.value = strings.ToLower(c.Value)
		}
	case "regex":
		if c.Value == "" {
			return nil, fmt.Errorf("字段 %s 的正则不能为空", field)
		}
		pat := c.Value
		if c.IgnoreCase {
			pat = "(?i)" + pat
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("字段 %s 正则非法: %w", field, err)
		}
		out.re = re
	case "cidr":
		p, err := parseCIDR(c.Value)
		if err != nil {
			return nil, fmt.Errorf("字段 %s 的 CIDR 非法: %w", field, err)
		}
		out.prefix = p
	case "in":
		for _, part := range strings.Split(c.Value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if c.IgnoreCase {
				part = strings.ToLower(part)
			}
			out.in = append(out.in, part)
		}
		if len(out.in) == 0 {
			return nil, fmt.Errorf("字段 %s 的 in 列表为空", field)
		}
	case "gt", "ge", "lt", "le":
		n, err := strconv.ParseFloat(strings.TrimSpace(c.Value), 64)
		if err != nil {
			return nil, fmt.Errorf("字段 %s 的数值比较值非法: %w", field, err)
		}
		out.num = n
	case "exists":
		if !strings.EqualFold(strings.TrimSpace(c.Value), "true") && !strings.EqualFold(strings.TrimSpace(c.Value), "false") {
			return nil, fmt.Errorf("字段 %s 的 exists 值只能是 true/false", field)
		}
		out.value = strings.ToLower(strings.TrimSpace(c.Value))
	default:
		return nil, fmt.Errorf("不支持的操作符: %s", op)
	}
	return out, nil
}

// validField 判断字段是否受支持。header./cookie./arg./query. 前缀允许任意后缀。
func validField(field string) bool {
	switch field {
	case "client_ip", "country", "region", "host", "method", "path", "query", "ua",
		"referer", "content_type", "body":
		return true
	}
	for _, p := range []string{"header.", "cookie.", "arg.", "query."} {
		if strings.HasPrefix(field, p) && len(field) > len(p) {
			return true
		}
	}
	return false
}

// empty 判断节点是否没有任何有效条件。
func (g *compiledGroup) empty() bool {
	return g == nil || (len(g.items) == 0 && len(g.groups) == 0)
}

// hasBodyDependency 判断条件树是否引用了请求体字段 body。
// 参数：无；返回 bool：任一叶子条件用到 body 即为 true。
func (g *compiledGroup) hasBodyDependency() bool {
	if g == nil {
		return false
	}
	for _, it := range g.items {
		if it.field == "body" {
			return true
		}
	}
	for _, sub := range g.groups {
		if sub.hasBodyDependency() {
			return true
		}
	}
	return false
}

// match 对条件树求值：AND 需要全部成立，OR 只需一个成立。
func (g *compiledGroup) match(req *Request) bool {
	if g.empty() {
		return false
	}
	if g.logic == "or" {
		for _, item := range g.items {
			if item.match(req) {
				return true
			}
		}
		for _, sub := range g.groups {
			if sub.match(req) {
				return true
			}
		}
		return false
	}
	for _, item := range g.items {
		if !item.match(req) {
			return false
		}
	}
	for _, sub := range g.groups {
		if !sub.match(req) {
			return false
		}
	}
	return true
}

// match 对单个条件求值。
func (c *compiledCondition) match(req *Request) bool {
	value, present := c.resolve(req)
	if c.op == "exists" {
		want := c.value == "true"
		return present == want
	}
	if c.ignoreCase {
		value = strings.ToLower(value)
	}
	switch c.op {
	case "eq":
		return value == c.value
	case "ne":
		return value != c.value
	case "contains":
		return strings.Contains(value, c.value)
	case "prefix":
		return strings.HasPrefix(value, c.value)
	case "suffix":
		return strings.HasSuffix(value, c.value)
	case "regex":
		return c.re != nil && c.re.MatchString(value)
	case "cidr":
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		return err == nil && c.prefix != nil && c.prefix.Contains(addr.Unmap())
	case "in":
		for _, v := range c.in {
			if value == v {
				return true
			}
		}
		return false
	case "gt", "ge", "lt", "le":
		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return false
		}
		switch c.op {
		case "gt":
			return n > c.num
		case "ge":
			return n >= c.num
		case "lt":
			return n < c.num
		default:
			return n <= c.num
		}
	}
	return false
}

// resolve 把字段名解析成实际值，并返回字段是否存在。
func (c *compiledCondition) resolve(req *Request) (string, bool) {
	switch c.field {
	case "client_ip":
		return req.ClientIP, req.ClientIP != ""
	case "country":
		return req.Country, req.Country != ""
	case "region":
		return req.Region, req.Region != ""
	case "host":
		return req.Host, req.Host != ""
	case "method":
		return req.Method, req.Method != ""
	case "path":
		return req.Path, true
	case "query":
		return req.Query, req.Query != ""
	case "ua":
		return req.UA, req.UA != ""
	case "referer":
		return req.Referer, req.Referer != ""
	case "content_type":
		return req.ContentType, req.ContentType != ""
	case "body":
		return req.Body, req.Body != ""
	}
	if strings.HasPrefix(c.field, "header.") {
		name := strings.TrimPrefix(c.field, "header.")
		vs, ok := req.Header[http.CanonicalHeaderKey(name)]
		if !ok || len(vs) == 0 {
			return "", false
		}
		return strings.Join(vs, ","), true
	}
	if strings.HasPrefix(c.field, "cookie.") {
		name := strings.TrimPrefix(c.field, "cookie.")
		v, ok := req.Cookie[name]
		return v, ok
	}
	if strings.HasPrefix(c.field, "arg.") {
		name := strings.TrimPrefix(c.field, "arg.")
		v, ok := req.Args[name]
		return v, ok
	}
	if strings.HasPrefix(c.field, "query.") {
		name := strings.TrimPrefix(c.field, "query.")
		v, ok := req.Args[name]
		return v, ok
	}
	return "", false
}

// parseCIDR 解析 CIDR；单个 IP 自动补 /32 或 /128。
func parseCIDR(v string) (*netip.Prefix, error) {
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
