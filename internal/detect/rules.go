package detect

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"gowaf/internal/store/model"
)

// Target 是检测目标位掩码。
type Target uint8

// 检测目标。
const (
	TargetPath Target = 1 << iota
	TargetQuery
	TargetBody
	TargetHeader
	TargetCookie
	TargetUA
)

var targetNames = map[string]Target{
	"path":   TargetPath,
	"query":  TargetQuery,
	"body":   TargetBody,
	"header": TargetHeader,
	"cookie": TargetCookie,
	"ua":     TargetUA,
}

// parseTargets 解析 "path,query,body" 形式。
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
	ID       string
	Name     string
	Category string
	Score    int
	Targets  Target
	Keywords []string
	Action   string
	Priority int
	re       *regexp.Regexp
}

// ruleSet 是编译后的不可变快照。
type ruleSet struct {
	rules     []*Rule
	keywordAC *AC
	byTarget  map[Target][]int
	noKeyword []int // 无关键词的规则，必须逐条跑
	version   string
}

// RuleSetInfo 给控制面展示的统计。
type RuleSetInfo struct {
	Rules    int    `json:"rules"`
	Keywords int    `json:"keywords"`
	Version  string `json:"version"`
}

// Compile 把数据库规则编译成快照。任何一条编译失败都会整体失败（fail-safe：
// 调用方保留旧快照，不会把规则集替换成残缺版本）。
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

	rs.version = fmt.Sprintf("%d-%d", len(rs.rules), time.Now().UnixMilli())
	return rs, nil
}

// reindex 排序后重建索引（排序改变了下标）。
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
func (rs *ruleSet) Info() RuleSetInfo {
	if rs == nil {
		return RuleSetInfo{}
	}
	return RuleSetInfo{Rules: len(rs.rules), Keywords: rs.keywordAC.Size(), Version: rs.version}
}

// Hit 是一条命中。
type Hit struct {
	RuleID   string `json:"ruleId"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Score    int    `json:"score"`
	Target   string `json:"target"`
	Evidence string `json:"evidence"`
}

// Result 是一次检测的结论。
type Result struct {
	Score      int      `json:"score"`
	Action     string   `json:"action"` // pass / log / challenge / block / ban
	Ban        bool     `json:"-"`      // 命中了 action=ban 的规则：一次即封，不走评分
	Hits       []Hit    `json:"hits"`
	Categories []string `json:"categories"`
	RuleIDs    []string `json:"ruleIds"`
	ElapsedUS  int64    `json:"elapsedUs"`
}

// Input 是待检测请求的抽象，由数据面填充。
type Input struct {
	Method  string
	Host    string
	Path    string
	Query   string
	Body    string
	UA      string
	Referer string
	Headers string
	Cookies string
}

// Engine 持有不可变规则快照，读路径无锁。
type Engine struct {
	rules          atomic.Pointer[ruleSet]
	mode           atomic.Value // string
	blockScore     atomic.Int32
	challengeScore atomic.Int32
}

// NewEngine 创建引擎。
func NewEngine(blockScore, challengeScore int, mode string) *Engine {
	e := &Engine{}
	e.blockScore.Store(int32(blockScore))
	e.challengeScore.Store(int32(challengeScore))
	e.mode.Store(mode)
	e.rules.Store(&ruleSet{byTarget: map[Target][]int{}})
	return e
}

// Reload 原子替换规则集；编译失败返回错误且保留旧快照。
func (e *Engine) Reload(rows []model.Rule) error {
	rs, err := Compile(rows)
	if err != nil {
		return err
	}
	e.rules.Store(rs)
	return nil
}

// SetMode 切换 observe / protect。
func (e *Engine) SetMode(mode string) {
	if mode != "observe" && mode != "protect" {
		return
	}
	e.mode.Store(mode)
}

// SetScores 热更新拦截/校验阈值（设置页保存后经 Reload 调用）。
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
func (e *Engine) Scores() (block, challenge int) {
	return int(e.blockScore.Load()), int(e.challengeScore.Load())
}

// Mode 返回当前模式。
func (e *Engine) Mode() string {
	if v, ok := e.mode.Load().(string); ok {
		return v
	}
	return "observe"
}

// Info 返回规则集统计。
func (e *Engine) Info() RuleSetInfo {
	return e.rules.Load().Info()
}

// Inspect 执行检测。纯函数式：不触碰网络与磁盘。
func (e *Engine) Inspect(in *Input) Result {
	start := time.Now()
	rs := e.rules.Load()
	res := Result{Hits: []Hit{}, Categories: []string{}, RuleIDs: []string{}}

	texts := map[Target]string{
		TargetPath:   Normalize(in.Path),
		TargetQuery:  Normalize(in.Query),
		TargetBody:   Normalize(Truncate(in.Body, 256*1024)),
		TargetHeader: Normalize(in.Headers + " " + in.Referer),
		TargetCookie: Normalize(in.Cookies),
		TargetUA:     Normalize(in.UA),
	}

	matched := make(map[string]struct{}, 4)
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
func (e *Engine) decide(score int) string {
	if score <= 0 {
		return "pass"
	}
	verdict := "pass"
	switch {
	case score >= int(e.blockScore.Load()):
		verdict = "block"
	case score >= int(e.challengeScore.Load()):
		verdict = "challenge"
	default:
		verdict = "log"
	}
	if e.Mode() == "observe" {
		// 观察模式：只记录不拦截。这是上线前 3~7 天的保命开关。
		return "log"
	}
	return verdict
}

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
	Method  string            `json:"method" binding:"required"`
	Path    string            `json:"path" binding:"required"`
	Query   string            `json:"query"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers"`
}

// Test 用模拟请求跑一遍检测，不转发、不落库。
func (e *Engine) Test(cfg TestConfig) Result {
	headers := ""
	ua := ""
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
