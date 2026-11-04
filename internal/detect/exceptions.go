package detect

import (
	"strings"
	"sync/atomic"
)

// Exception 是编译后的规则例外。任何一条命中即跳过该次规则命中。
//
// 设计动机：自研 WAF 最常见的死法是「误报 → 只能整组关规则」。
// 例外粒度做到 站点 + 路径 + 规则 + 参数，才能既保住防护又放过业务。
type Exception struct {
	Site       string
	RuleID     string
	Category   string
	PathPrefix string
	Param      string
}

// exceptionSet 是不可变快照，读路径无锁。
type exceptionSet struct {
	items []Exception
}

func (ex *exceptionSet) empty() bool { return ex == nil || len(ex.items) == 0 }

// Match 判断这次命中是否被例外放过。
func (ex *exceptionSet) Match(site, path, ruleID, category string, params []string) bool {
	if ex.empty() {
		return false
	}
	for _, e := range ex.items {
		if e.Site != "" && e.Site != site {
			continue
		}
		if e.RuleID != "" && e.RuleID != ruleID {
			continue
		}
		if e.Category != "" && e.Category != category {
			continue
		}
		if e.PathPrefix != "" && !strings.HasPrefix(path, e.PathPrefix) {
			continue
		}
		if e.Param != "" && !containsString(params, e.Param) {
			continue
		}
		return true
	}
	return false
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// maxFilterScore 是过滤后重算分数的上限，与 CRS 的 300 分封顶保持一致。
const maxFilterScore = 300

// FilterByException 按例外快照过滤结果里的命中并重算分数、命中规则与类型。
//
// 供第二引擎（CRS）复用同一份例外：用户已经把某条误报例外掉，
// 不该因为 CRS 命中同一个特征又被拦一次，否则例外形同虚设。
func (e *Engine) FilterByException(res *Result, site, path string, params []string) {
	if res == nil || len(res.Hits) == 0 {
		return
	}
	ex := e.exceptions.Load()
	if ex.empty() {
		return
	}
	kept := res.Hits[:0]
	score := 0
	var ruleIDs, cats []string
	seenRule := make(map[string]struct{}, len(res.RuleIDs))
	seenCat := make(map[string]struct{}, len(res.Categories))
	for _, h := range res.Hits {
		if ex.Match(site, path, h.RuleID, h.Category, params) {
			continue
		}
		kept = append(kept, h)
		score += h.Score
		if score > maxFilterScore {
			score = maxFilterScore
		}
		if _, ok := seenRule[h.RuleID]; !ok && h.RuleID != "" {
			seenRule[h.RuleID] = struct{}{}
			ruleIDs = append(ruleIDs, h.RuleID)
		}
		if _, ok := seenCat[h.Category]; !ok && h.Category != "" {
			seenCat[h.Category] = struct{}{}
			cats = append(cats, h.Category)
		}
	}
	res.Hits = kept
	res.Score = score
	res.RuleIDs = ruleIDs
	res.Categories = cats
}

// SetExceptions 原子替换例外快照。
func (e *Engine) SetExceptions(list []Exception) {
	cp := make([]Exception, 0, len(list))
	for _, it := range list {
		cp = append(cp, it)
	}
	e.exceptions.Store(&exceptionSet{items: cp})
}

// ExceptionCount 返回当前例外条数。
func (e *Engine) ExceptionCount() int {
	if ex := e.exceptions.Load(); ex != nil {
		return len(ex.items)
	}
	return 0
}

type hitStat struct {
	count atomic.Int64
	last  atomic.Int64
}

// HitStat 是给后台任务刷库用的命中统计。
type HitStat struct {
	RuleID string
	Count  int64
	LastMS int64
}

// recordHit 累加规则命中次数。热路径，只做两次原子操作。
func (e *Engine) recordHit(id string, nowMS int64) {
	v, ok := e.hits.Load(id)
	if !ok {
		v, _ = e.hits.LoadOrStore(id, &hitStat{})
	}
	st := v.(*hitStat)
	st.count.Add(1)
	st.last.Store(nowMS)
}

// DrainHits 取走并清零累计，供后台任务定期落库。
func (e *Engine) DrainHits() []HitStat {
	out := make([]HitStat, 0, 32)
	e.hits.Range(func(k, v any) bool {
		st := v.(*hitStat)
		n := st.count.Swap(0)
		if n > 0 {
			out = append(out, HitStat{RuleID: k.(string), Count: n, LastMS: st.last.Load()})
		}
		return true
	})
	return out
}
