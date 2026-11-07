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
	Site       string // 限定的站点（Host），空表示不限站点
	RuleID     string // 限定的规则 ID，空表示不限规则
	Category   string // 限定的规则类别（如 sqli/xss），空表示不限类别
	PathPrefix string // 限定的请求路径前缀，空表示不限路径
	Param      string // 限定的参数名（须出现在命中的参数列表中），空表示不限参数
}

// exceptionSet 是不可变快照，读路径无锁。
type exceptionSet struct {
	items []Exception // 例外条目列表，快照发布后只读
}

// empty 判断例外集是否为空（nil 接收者也视为空）。
// 返回 bool：没有例外条目时为 true。
func (ex *exceptionSet) empty() bool { return ex == nil || len(ex.items) == 0 }

// Match 判断这次命中是否被例外放过。条目中各字段之间是"与"关系：
// 非空字段必须全部满足（相等/前缀/包含），全部满足的条目即豁免本次命中。
// 参数 site：请求所属站点（Host）；参数 path：请求路径；参数 ruleID：命中的规则 ID；
// 参数 category：命中的规则类别；参数 params：本次命中涉及的参数名列表。
// 返回 bool：true 表示该命中被例外豁免。
func (ex *exceptionSet) Match(site, path, ruleID, category string, params []string) bool {
	if ex.empty() {
		return false
	}
	for _, e := range ex.items {
		// 字段为空表示该项不限定；非空则必须匹配，不匹配就试下一条例外
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

// containsString 判断列表中是否包含目标字符串。
// 参数 list：待查找的字符串列表；参数 want：目标字符串。
// 返回 bool：存在与 want 相等的元素时为 true。
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
// 参数 res：待过滤的检测结果，Hits/Score/RuleIDs/Categories 会被就地改写；
// 参数 site：请求所属站点；参数 path：请求路径；参数 params：命中涉及的参数名列表。
// 无返回值。
func (e *Engine) FilterByException(res *Result, site, path string, params []string) {
	if res == nil || len(res.Hits) == 0 {
		return
	}
	ex := e.exceptions.Load()
	if ex.empty() {
		return
	}
	// 原地过滤：复用 Hits 底层数组，被豁免的命中直接覆盖丢弃
	kept := res.Hits[:0]
	score := 0
	var ruleIDs, cats []string
	// 去重集合：重算 RuleIDs/Categories 时保持原有的去重与顺序语义
	seenRule := make(map[string]struct{}, len(res.RuleIDs))
	seenCat := make(map[string]struct{}, len(res.Categories))
	for _, h := range res.Hits {
		if ex.Match(site, path, h.RuleID, h.Category, params) {
			continue
		}
		kept = append(kept, h)
		score += h.Score
		// 重算总分时同样封顶，与 CRS 原始评分口径一致
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
// 参数 list：新的例外列表，内部会逐条拷贝，外部后续修改不影响已发布快照。
// 无返回值。
func (e *Engine) SetExceptions(list []Exception) {
	cp := make([]Exception, 0, len(list))
	for _, it := range list {
		cp = append(cp, it)
	}
	e.exceptions.Store(&exceptionSet{items: cp})
}

// ExceptionCount 返回当前例外条数。
// 返回 int：当前快照中的例外条数，无快照时为 0。
func (e *Engine) ExceptionCount() int {
	if ex := e.exceptions.Load(); ex != nil {
		return len(ex.items)
	}
	return 0
}

// hitStat 单条规则的运行期命中计数，原子字段读写无锁。
type hitStat struct {
	count atomic.Int64 // 累计命中次数
	last  atomic.Int64 // 最近一次命中的毫秒时间戳
}

// HitStat 是给后台任务刷库用的命中统计。
type HitStat struct {
	RuleID string // 规则 ID
	Count  int64  // 取走时的累计命中次数
	LastMS int64  // 最近一次命中的毫秒时间戳
}

// recordHit 累加规则命中次数并刷新最近命中时间。热路径，只做两次原子操作。
// 参数 id：规则 ID；参数 nowMS：当前毫秒时间戳。
// 无返回值。
func (e *Engine) recordHit(id string, nowMS int64) {
	// 先 Load 命中常规情况；不存在时再用 LoadOrStore 兜住并发首次插入
	v, ok := e.hits.Load(id)
	if !ok {
		v, _ = e.hits.LoadOrStore(id, &hitStat{})
	}
	st := v.(*hitStat)
	st.count.Add(1)
	st.last.Store(nowMS)
}

// DrainHits 取走并清零累计的命中统计，供后台任务定期落库。
// 返回 []HitStat：自上次调用以来计数大于 0 的各规则命中统计。
func (e *Engine) DrainHits() []HitStat {
	out := make([]HitStat, 0, 32)
	e.hits.Range(func(k, v any) bool {
		st := v.(*hitStat)
		n := st.count.Swap(0) // 原子取走旧计数并清零，期间的新命中会计入下一轮
		if n > 0 {
			out = append(out, HitStat{RuleID: k.(string), Count: n, LastMS: st.last.Load()})
		}
		return true
	})
	return out
}
