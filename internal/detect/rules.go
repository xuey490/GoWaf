package detect

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gowaf/internal/store/model"
)

// Target 是检测目标位掩码。
type Target uint8

// 检测目标位掩码，从低位到高位依次为 path / query / body / header / cookie / ua。
const (
	TargetPath Target = 1 << iota
	TargetQuery
	TargetBody
	TargetHeader
	TargetCookie
	TargetUA
)

// targetNames 是目标名到掩码的映射，供 parseTargets 解析规则的 targets 字段。
var targetNames = map[string]Target{
	"path":   TargetPath,
	"query":  TargetQuery,
	"body":   TargetBody,
	"header": TargetHeader,
	"cookie": TargetCookie,
	"ua":     TargetUA,
}

// parseTargets 解析 "path,query,body" 形式的目标串。
// 参数 s：逗号分隔的目标名列表，忽略大小写与空白项。
// 返回 Target：目标位掩码，未给出任何有效目标时默认 path|query|body；
// 含未知目标名时返回 error。
func parseTargets(s string) (Target, error) {
	var mask Target
	for _, part := range strings.Split(s, ",") {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		t, ok := targetNames[p]
		if !ok {
			return 0, fmt.Errorf("未知检测目标 %q", p)
		}
		mask |= t
	}
	if mask == 0 {
		mask = TargetPath | TargetQuery | TargetBody
	}
	return mask, nil
}

// Rule 是编译后的规则。
type Rule struct {
	ID       string   // 规则唯一标识
	Name     string   // 规则名称，展示用
	Category string   // 分类（如 sqli/xss），用于面板统计与例外匹配
	Score    int      // 单次命中的加分，<=0 时编译期回退为 30
	Targets  Target   // 检测目标位掩码，path/query/body/header/cookie/ua 的组合
	Keywords []string // 归一化后的关键词，AC 预筛命中后再精确确认
	Action   string   // 命中后的动作：pass / log / challenge / block / ban
	Priority int      // 优先级，数字小的先匹配
	// LogOnly 为 true 时命中只记录、不计分（面板上的「观察」模式），
	// 用来先跑一段时间看误报，再决定是否真正拦截。
	LogOnly bool
	re      *regexp.Regexp // 编译后的正则；nil 表示纯关键词规则
}

// ruleSet 是编译后的不可变快照。
type ruleSet struct {
	rules     []*Rule          // 规则列表，已按 Priority 升序
	keywordAC *AC              // 全量关键词的 AC 自动机，用于 O(n) 预筛候选规则
	byTarget  map[Target][]int // 目标 -> 关心该目标的规则下标列表
	noKeyword []int            // 无关键词的规则，必须逐条跑
	version   string           // 快照版本（规则数-编译时间毫秒），供面板展示
}

// RuleSetInfo 给控制面展示的统计。
type RuleSetInfo struct {
	Rules    int    `json:"rules"`    // 编译后的规则总数
	Keywords int    `json:"keywords"` // 关键词总数（归一化去重后）
	Version  string `json:"version"`  // 快照版本标识
}

// Compile 把数据库规则编译成快照。
// 参数 rows：数据库读出的规则行，Enabled==0 的行跳过。
// 返回 *ruleSet：编译完成的不可变快照；任何一条解析或正则编译失败都整体失败
// （fail-safe：调用方保留旧快照，不会把规则集替换成残缺版本），此时返回 error。
func Compile(rows []model.Rule) (*ruleSet, error) {
	rs := &ruleSet{byTarget: map[Target][]int{}}
	var allKeywords []string

	for i := range rows {
		r := rows[i]
		if r.Enabled == 0 {
			continue
		}
		targets, err := parseTargets(r.Targets)
		if err != nil {
			return nil, fmt.Errorf("规则 %s: %w", r.ID, err)
		}
		rule := &Rule{
			ID:       r.ID,
			Name:     r.Name,
			Category: r.Category,
			Score:    r.Score,
			Targets:  targets,
			Action:   strings.ToLower(strings.TrimSpace(r.Action)),
			Priority: r.Priority,
		}
		if rule.Action == "" {
			rule.Action = "block"
		}
		if rule.Score <= 0 {
			rule.Score = 30
		}
		if strings.EqualFold(strings.TrimSpace(r.Mode), "log") {
			rule.LogOnly = true
		}

		// Keywords 列存的是 JSON 数组字符串，解析失败视为该规则配置错误
		if r.Keywords != "" {
			var kws []string
			if err := json.Unmarshal([]byte(r.Keywords), &kws); err != nil {
				return nil, fmt.Errorf("规则 %s 关键词解析失败: %w", r.ID, err)
			}
			for _, kw := range kws {
				nk := Normalize(kw)
				if nk == "" {
					continue
				}
				rule.Keywords = append(rule.Keywords, nk)
			}
			allKeywords = append(allKeywords, rule.Keywords...)
		}

		// Pattern 列为正则表达式；统一补 (?i) 前缀，保证大小写不敏感
		if r.Pattern != "" {
			pattern := r.Pattern
			if !strings.HasPrefix(pattern, "(?i)") {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("规则 %s 正则编译失败: %w", r.ID, err)
			}
			rule.re = re
		}

		idx := len(rs.rules)
		rs.rules = append(rs.rules, rule)
		// 无关键词但带正则的规则：AC 预筛覆盖不到，单独记录、逐条执行
		if len(rule.Keywords) == 0 && rule.re != nil {
			rs.noKeyword = append(rs.noKeyword, idx)
		}
		for _, t := range []Target{TargetPath, TargetQuery, TargetBody, TargetHeader, TargetCookie, TargetUA} {
			if rule.Targets&t != 0 {
				rs.byTarget[t] = append(rs.byTarget[t], idx)
			}
		}
	}

	rs.keywordAC = NewAC(allKeywords)

	// 按 priority 升序（数字小的先匹配），再按 id 保证稳定
	sort.SliceStable(rs.rules, func(i, j int) bool { return rs.rules[i].Priority < rs.rules[j].Priority })
	rs.reindex()

	// 版本号 = 规则数-编译时间戳（毫秒），仅供面板展示
	rs.version = fmt.Sprintf("%d-%d", len(rs.rules), time.Now().UnixMilli())
	return rs, nil
}

// reindex 排序后重建索引（排序改变了下标）。
// 无参数；按当前 rules 顺序重算 byTarget 映射与 noKeyword 列表，无返回值。
func (rs *ruleSet) reindex() {
	rs.byTarget = map[Target][]int{}
	rs.noKeyword = rs.noKeyword[:0]
	for i, r := range rs.rules {
		if len(r.Keywords) == 0 && r.re != nil {
			rs.noKeyword = append(rs.noKeyword, i)
		}
		for _, t := range []Target{TargetPath, TargetQuery, TargetBody, TargetHeader, TargetCookie, TargetUA} {
			if r.Targets&t != 0 {
				rs.byTarget[t] = append(rs.byTarget[t], i)
			}
		}
	}
}

// Info 返回统计信息。
// 无参数；返回 RuleSetInfo（规则数、关键词数、版本），接收者为 nil 时返回零值。
func (rs *ruleSet) Info() RuleSetInfo {
	if rs == nil {
		return RuleSetInfo{}
	}
	return RuleSetInfo{Rules: len(rs.rules), Keywords: rs.keywordAC.Size(), Version: rs.version}
}

// Hit 是一条命中。
type Hit struct {
	RuleID   string `json:"ruleId"`   // 命中的规则标识（CRS 命中带 crs-/seclang- 前缀）
	Name     string `json:"name"`     // 规则名称或 CRS 告警消息
	Category string `json:"category"` // 命中分类
	Score    int    `json:"score"`    // 本次命中的加分
	Target   string `json:"target"`   // 命中部位：path/query/body/header/cookie/ua/crs
	Evidence string `json:"evidence"` // 命中位置附近的证据片段
}

// Result 是一次检测的结论。
type Result struct {
	Score      int      `json:"score"`      // 累计得分
	Action     string   `json:"action"`     // pass / log / challenge / block / ban
	Ban        bool     `json:"-"`          // 命中了 action=ban 的规则：一次即封，不走评分
	Hits       []Hit    `json:"hits"`       // 命中明细
	Categories []string `json:"categories"` // 命中规则的去重分类列表
	RuleIDs    []string `json:"ruleIds"`    // 命中规则标识列表
	ElapsedUS  int64    `json:"elapsedUs"`  // 本次检测耗时（微秒）
}

// Input 是待检测请求的抽象，由数据面填充。
type Input struct {
	Method  string // HTTP 方法
	Host    string // Host 头里的站点域名
	Path    string // 请求路径（不含 query）
	Query   string // 查询串（不含问号）
	Body    string // 请求体文本
	UA      string // User-Agent
	Referer string // Referer 头
	Headers string // 请求头拼成的 "K: v" 串，空格分隔
	Cookies string // Cookie 头原文
	// Site 是命中的站点名，供规则例外按站点匹配。
	Site string
	// Params 是 query/body 里出现过的参数名，供例外按参数名匹配。
	Params []string
}

// Engine 持有不可变规则快照，读路径无锁。
type Engine struct {
	rules          atomic.Pointer[ruleSet]      // 当前规则快照；热更新时整体原子替换
	mode           atomic.Value                 // string，运行模式 observe / protect
	blockScore     atomic.Int32                 // 拦截阈值，得分达到即 block
	challengeScore atomic.Int32                 // 校验阈值，得分达到即 challenge（人机验证）
	exceptions     atomic.Pointer[exceptionSet] // 规则例外集，整体原子替换
	hits           sync.Map                     // ruleID -> *hitStat，命中统计（并发安全）
}

// NewEngine 创建引擎。
// 参数 blockScore：拦截阈值；challengeScore：校验（人机验证）阈值；mode：初始运行模式。
// 返回 *Engine：带空规则集与空例外集的引擎实例。
func NewEngine(blockScore, challengeScore int, mode string) *Engine {
	e := &Engine{}
	e.blockScore.Store(int32(blockScore))
	e.challengeScore.Store(int32(challengeScore))
	e.mode.Store(mode)
	e.rules.Store(&ruleSet{byTarget: map[Target][]int{}})
	e.exceptions.Store(&exceptionSet{})
	return e
}

// Reload 原子替换规则集。
// 参数 rows：数据库读出的最新规则行。
// 返回 error：编译失败时非空且保留旧快照；成功返回 nil。
func (e *Engine) Reload(rows []model.Rule) error {
	rs, err := Compile(rows)
	if err != nil {
		return err
	}
	e.rules.Store(rs)
	return nil
}

// SetMode 切换 observe / protect。
// 参数 mode：目标模式；非法值直接忽略、保持原模式。无返回值。
func (e *Engine) SetMode(mode string) {
	if mode != "observe" && mode != "protect" {
		return
	}
	e.mode.Store(mode)
}

// SetScores 热更新拦截/校验阈值（设置页保存后经 Reload 调用）。
// 参数 block、challenge：新阈值；非正数表示该项不更新。无返回值。
// 此前阈值只在 NewEngine 时从配置文件读取，设置页改了只落库不生效。
func (e *Engine) SetScores(block, challenge int) {
	if block > 0 {
		e.blockScore.Store(int32(block))
	}
	if challenge > 0 {
		e.challengeScore.Store(int32(challenge))
	}
}

// Scores 返回当前阈值。
// 无参数；返回 block：拦截阈值，challenge：校验阈值。
func (e *Engine) Scores() (block, challenge int) {
	return int(e.blockScore.Load()), int(e.challengeScore.Load())
}

// Mode 返回当前模式。
// 无参数；值缺失时回退 "observe"。
func (e *Engine) Mode() string {
	if v, ok := e.mode.Load().(string); ok {
		return v
	}
	return "observe"
}

// Info 返回规则集统计。
// 无参数；返回当前快照的规则数、关键词数与版本。
func (e *Engine) Info() RuleSetInfo {
	return e.rules.Load().Info()
}

// Inspect 执行检测。纯函数式：不触碰网络与磁盘。
// 参数 in：已标准化的请求数据。
// 返回 Result：累计得分、命中明细与处置动作；同一规则只记一次命中。
func (e *Engine) Inspect(in *Input) Result {
	start := time.Now()
	rs := e.rules.Load()
	res := Result{Hits: []Hit{}, Categories: []string{}, RuleIDs: []string{}}

	// 各部位文本归一化后作为检测输入；body 截断到 256KB，防止超大包拖垮正则
	texts := map[Target]string{
		TargetPath:   Normalize(in.Path),
		TargetQuery:  Normalize(in.Query),
		TargetBody:   Normalize(Truncate(in.Body, 256*1024)),
		TargetHeader: Normalize(in.Headers + " " + in.Referer),
		TargetCookie: Normalize(in.Cookies),
		TargetUA:     Normalize(in.UA),
	}

	ex := e.exceptions.Load()
	matched := make(map[string]struct{}, 4) // 已命中的规则 ID：同一规则跨部位只记一次
	for t, text := range texts {
		if text == "" {
			continue
		}
		candidateIdx := rs.candidates(t, text)
		for _, idx := range candidateIdx {
			rule := rs.rules[idx]
			if _, ok := matched[rule.ID]; ok {
				continue
			}
			evidence, ok := rule.match(text)
			if !ok {
				continue
			}
			matched[rule.ID] = struct{}{}
			if ex.Match(in.Site, in.Path, rule.ID, rule.Category, in.Params) {
				continue
			}
			e.recordHit(rule.ID, time.Now().UnixMilli())
			if rule.LogOnly {
				// 观察中的规则：记录命中但不计分，避免影响处置动作。
				res.Hits = append(res.Hits, Hit{
					RuleID: rule.ID, Name: rule.Name, Category: rule.Category,
					Score: 0, Target: targetLabel(t), Evidence: evidence,
				})
				res.RuleIDs = append(res.RuleIDs, rule.ID)
				res.Categories = appendUnique(res.Categories, rule.Category)
				continue
			}
			res.Score += rule.Score
			if rule.Action == "ban" {
				// 高危规则：命中即封。只打标记，最终动作在下面统一决定
				//（observe 模式下会降级为评分结果，保持"观察模式不拦截"语义）。
				res.Ban = true
			}
			res.Hits = append(res.Hits, Hit{
				RuleID:   rule.ID,
				Name:     rule.Name,
				Category: rule.Category,
				Score:    rule.Score,
				Target:   targetLabel(t),
				Evidence: evidence,
			})
			res.RuleIDs = append(res.RuleIDs, rule.ID)
			res.Categories = appendUnique(res.Categories, rule.Category)
		}
	}

	res.Action = e.decide(res.Score)
	if res.Ban && e.Mode() != "observe" {
		// action=ban 的规则命中即封：一次就封禁来源 IP，不走评分与重试。
		// observe 模式下保持 decide() 的结果（只记录不拦截）。
		res.Action = "ban"
	}
	res.ElapsedUS = time.Since(start).Microseconds()
	return res
}

// candidates 返回该目标可能命中的规则下标：AC 命中的 + 无关键词需逐条跑的。
// 参数 t：检测目标；text：归一化后的部位文本。
// 返回 []int：候选规则在 rs.rules 中的下标，升序返回。
func (rs *ruleSet) candidates(t Target, text string) []int {
	targetRules := rs.byTarget[t]
	if len(targetRules) == 0 {
		return nil
	}
	hitSet := make(map[int]struct{}, len(targetRules))

	// 用 AC 找出命中的关键词，再映射回规则
	for _, kwIdx := range rs.keywordAC.Match(text) {
		kw := rs.keywordAC.keywords[kwIdx]
		for _, ri := range targetRules {
			rule := rs.rules[ri]
			if len(rule.Keywords) == 0 {
				continue
			}
			for _, k := range rule.Keywords {
				if k == kw {
					hitSet[ri] = struct{}{}
					break
				}
			}
		}
	}
	// 无关键词的规则只能逐条跑
	for _, ri := range rs.noKeyword {
		if rs.rules[ri].Targets&t != 0 {
			hitSet[ri] = struct{}{}
		}
	}

	out := make([]int, 0, len(hitSet))
	for ri := range hitSet {
		out = append(out, ri)
	}
	sort.Ints(out)
	return out
}

// match 判断规则是否命中该文本，并给出证据片段。
// 参数 text：归一化后的部位文本。
// 返回 string：证据片段（命中位置前后各扩 20 字符）；返回 bool：是否命中。
func (r *Rule) match(text string) (string, bool) {
	if r.re != nil {
		loc := r.re.FindStringIndex(text)
		if loc == nil {
			return "", false
		}
		start := loc[0] - 20
		if start < 0 {
			start = 0
		}
		end := loc[1] + 20
		if end > len(text) {
			end = len(text)
		}
		return text[start:end], true
	}
	// 纯关键词规则：AC 命中已由 candidates 保证，这里再确认一次
	for _, kw := range r.Keywords {
		if idx := strings.Index(text, kw); idx >= 0 {
			start := idx - 20
			if start < 0 {
				start = 0
			}
			end := idx + len(kw) + 20
			if end > len(text) {
				end = len(text)
			}
			return text[start:end], true
		}
	}
	return "", false
}

// decide 依据评分与模式给出动作。
// 参数 score：累计得分。返回 string：pass / log / challenge / block。
func (e *Engine) decide(score int) string {
	return Decide(score, int(e.blockScore.Load()), int(e.challengeScore.Load()), e.Mode())
}

// Decide 是评分→动作的纯函数，数据面合并多个引擎的分数后也用它重判。
// 参数 score：累计得分；block：拦截阈值；challenge：校验阈值；mode：observe / protect。
// 返回 string：pass（score<=0）/ block（>=block）/ challenge（>=challenge）/ log。
func Decide(score, block, challenge int, mode string) string {
	if score <= 0 {
		return "pass"
	}
	if block <= 0 {
		block = 60 // 未配置时用默认拦截阈值
	}
	if challenge <= 0 || challenge >= block {
		challenge = block / 2 // 校验阈值必须严格低于拦截阈值
	}
	verdict := "log"
	switch {
	case score >= block:
		verdict = "block"
	case score >= challenge:
		verdict = "challenge"
	}
	if mode == "observe" {
		// 观察模式：只记录不拦截。这是上线前 3~7 天的保命开关。
		return "log"
	}
	return verdict
}

// targetLabel 返回检测目标的展示名。
// 参数 t：目标掩码。返回 string：小写目标名；未知掩码返回 "unknown"。
func targetLabel(t Target) string {
	switch t {
	case TargetPath:
		return "path"
	case TargetQuery:
		return "query"
	case TargetBody:
		return "body"
	case TargetHeader:
		return "header"
	case TargetCookie:
		return "cookie"
	case TargetUA:
		return "ua"
	}
	return "unknown"
}

// appendUnique 向列表追加元素；元素已存在则原样返回。
// 参数 list：原列表；v：待追加元素。返回 []string：去重后的列表。
func appendUnique(list []string, v string) []string {
	for _, item := range list {
		if item == v {
			return list
		}
	}
	return append(list, v)
}

// TestConfig 规则测试器的输入。
type TestConfig struct {
	Method  string            `json:"method" binding:"required"` // HTTP 方法
	Path    string            `json:"path" binding:"required"`   // 请求路径
	Query   string            `json:"query"`                     // 查询串
	Body    string            `json:"body"`                      // 请求体
	Headers map[string]string `json:"headers"`                   // 请求头键值对
}

// Test 用模拟请求跑一遍检测，不转发、不落库。
// 参数 cfg：控制台提交的模拟请求（方法、路径、查询串、请求体、请求头）。
// 返回 Result：与真实检测同构的结果。
func (e *Engine) Test(cfg TestConfig) Result {
	headers := ""
	ua := ""
	// 把请求头拼成 "K: v" 串，同时摘出 User-Agent 供 UA 部位检测
	for k, v := range cfg.Headers {
		headers += k + ": " + v + " "
		if strings.EqualFold(k, "User-Agent") {
			ua = v
		}
	}
	return e.Inspect(&Input{
		Method:  cfg.Method,
		Path:    cfg.Path,
		Query:   cfg.Query,
		Body:    cfg.Body,
		UA:      ua,
		Headers: headers,
	})
}
