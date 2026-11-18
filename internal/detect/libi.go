package detect

import (
	"sync/atomic"
	"time"

	libinjection "github.com/corazawaf/libinjection-go"
)

// LibiEngine 是基于 libinjection 的语义检测通道（第三引擎）。
//
// 自研规则与 CRS 都是"特征匹配"：攻击者换个写法就可能绕过。libinjection 用
// 词法+指纹的方式判定"这段输入的结构是不是 SQL/XSS"，对变形 payload 的泛化
// 能力远强于正则，也是雷池语义分析思路在开源组件里的最小可用近似。
//
// 定位与取值策略：
//  1. 它是叠加的第三引擎，与 CRS 并列跑，分数进统一评分体系；
//  2. 命中即"高置信证据"，单条分数默认压在挑战阈值附近（45/35 < 默认 block 60），
//     拦不拦交给全局模式与阈值决定，避免语义误报直接打死正常访客；
//  3. 引擎不持有状态，Inspect 天然并发安全；开关与分数由 Reload 时整体替换。
type LibiEngine struct {
	enabled   atomic.Bool // 是否启用本通道
	sqliScore atomic.Int32
	xssScore  atomic.Int32
}

// libiMaxScan libinjection 单部位扫描的文本上限（字节）。
// libinjection 是 DFA 实现，线性耗时，但 256KB 的 body 全量喂进去依然浪费；
// 攻击 payload 几乎不可能出现在 32KB 之后，截断即可。
const libiMaxScan = 32 * 1024

// libi 默认分值：SQLi 比 XSS 略高（误报率更低、危害更大）。
const (
	DefaultLibiSQLiScore = 45
	DefaultLibiXSSScore  = 35
)

// libi 规则标识与分类：规则 ID 带 libi- 前缀，事件页与例外规则均可按 ID 精确治理。
const (
	RuleIDLibiSQLi = "libi-sqli"
	RuleIDLibiXSS  = "libi-xss"
	CategorySQLi   = "sqli"
	CategoryXSS    = "xss"
)

// NewLibiEngine 创建语义检测通道。
// 参数 enabled：是否启用；sqliScore/xssScore：SQLi 与 XSS 命中的加分（<=0 时回退默认值）。
// 返回 *LibiEngine：空输入的引擎实例，等待 ReloadSettings 装配参数。
func NewLibiEngine(enabled bool, sqliScore, xssScore int) *LibiEngine {
	e := &LibiEngine{}
	e.enabled.Store(enabled)
	if sqliScore <= 0 {
		sqliScore = DefaultLibiSQLiScore
	}
	if xssScore <= 0 {
		xssScore = DefaultLibiXSSScore
	}
	e.sqliScore.Store(int32(sqliScore))
	e.xssScore.Store(int32(xssScore))
	return e
}

// Enabled 报告通道是否启用。
// 无参数；返回 bool：启用返回 true。
func (e *LibiEngine) Enabled() bool { return e.enabled.Load() }

// Inspect 对 Query 与 Body（含解码变体）跑 libinjection 语义判定。
// 参数 in：已标准化的请求数据（与自研引擎共用同一 Input）。
// 返回 Result：命中明细与分数；未启用或无命中时返回零分空结果。
//
// 只扫 query/body 两个部位：path 的语义结构几乎不可能是 SQL/XSS，header/cookie
// 的语义判定误报代价高（登录 Cookie 里出现 "or" 一类的词太常见）。
// 解码变体（base64 解出、unicode 转义还原）一并扫描——语义通道是绕过面
// 最大的受益者；同一部位命中只记一次（原文命中后变体不再重复计分）。
func (e *LibiEngine) Inspect(in *Input) Result {
	start := time.Now()
	res := Result{Hits: []Hit{}, Categories: []string{}, RuleIDs: []string{}}
	if !e.enabled.Load() {
		return res
	}

	type scan struct {
		target string
		text   string
	}
	scanList := make([]scan, 0, 2+len(in.DecodedQuery)+len(in.DecodedBody))
	scanList = append(scanList, scan{"query", in.Query}, scan{"body", Truncate(in.Body, libiMaxScan)})
	for _, v := range in.DecodedQuery {
		scanList = append(scanList, scan{"query", Truncate(v, libiMaxScan)})
	}
	for _, v := range in.DecodedBody {
		scanList = append(scanList, scan{"body", Truncate(v, libiMaxScan)})
	}
	sqliSeen := false
	xssSeen := false
	for _, s := range scanList {
		if s.text == "" {
			continue
		}
		if !sqliSeen {
			if fp, ok := libiSQLi(s.text); ok {
				sqliSeen = true
				res.Score += int(e.sqliScore.Load())
				res.Hits = append(res.Hits, Hit{
					RuleID: RuleIDLibiSQLi, Name: "libinjection 语义识别 SQL 注入",
					Category: CategorySQLi, Score: int(e.sqliScore.Load()),
					Target: s.target, Evidence: "指纹 " + fp,
				})
				res.RuleIDs = append(res.RuleIDs, RuleIDLibiSQLi)
				res.Categories = appendUnique(res.Categories, CategorySQLi)
			}
		}
		if !xssSeen && libinjection.IsXSS(s.text) {
			xssSeen = true
			res.Score += int(e.xssScore.Load())
			res.Hits = append(res.Hits, Hit{
				RuleID: RuleIDLibiXSS, Name: "libinjection 语义识别 XSS",
				Category: CategoryXSS, Score: int(e.xssScore.Load()),
				Target: s.target, Evidence: Truncate(s.text, 120),
			})
			res.RuleIDs = append(res.RuleIDs, RuleIDLibiXSS)
			res.Categories = appendUnique(res.Categories, CategoryXSS)
		}
	}
	res.ElapsedUS = time.Since(start).Microseconds()
	return res
}

// libiSQLi 包装 libinjection.IsSQLi，返回命中与否与攻击指纹。
// 参数 s：待判定文本。
// 返回 (string, bool)：命中时返回指纹（如 "1sor"）与 true；未命中返回空串与 false。
// 异常兜底：libinjection 对超长/多字节输入理论上安全，但为防上游实现的边界问题，
// panic 一律按未命中处理（本通道是叠加证据，绝不能因它拖垮检测主链路）。
func libiSQLi(s string) (fp string, ok bool) {
	defer func() {
		if recover() != nil {
			fp, ok = "", false
		}
	}()
	hit, fingerprint := libinjection.IsSQLi(s)
	if !hit {
		return "", false
	}
	return fingerprint, true
}
