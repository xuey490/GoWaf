package detect

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corazaconf "github.com/corazawaf/coraza-coreruleset/coraza/v3"
	crsv4 "github.com/corazawaf/coraza-coreruleset/crs/v4"
	"github.com/corazawaf/coraza/v3"
	"github.com/corazawaf/coraza/v3/types"
	"github.com/jcchavezs/mergefs"

	"gowaf/internal/detect/crspatch"
)

// CRSEngine 是 OWASP CRS 的第二检测引擎（基于 Coraza）。
//
// 分工：
//   - 自研规则（rules 表）负责快、准、可解释的业务规则；
//   - CRS 负责广覆盖的通用攻击特征，随上游持续更新，不靠我们自己追 CVE。
//
// 两个引擎并行跑，分数相加后由同一个 decide() 判动作。
//
// 为什么不用「把 SecRule 翻译成我们的 YAML」：CRS 依赖变换链（t:urlDecodeUni、
// t:removeComments）、变量集合（ARGS_NAMES）、chain 与 TX 变量，翻译只能覆盖
// 30~40% 的简单规则，且丢掉变换链后等于筛子——所以直接嵌引擎。
type CRSEngine struct {
	waf      coraza.WAF
	mode     string // block / log
	paranoia int
	version  string
}

// CRSOptions 构造参数。
type CRSOptions struct {
	// Mode: block（SecRuleEngine On，命中阈值即拦截）/ log（DetectionOnly，只记录）
	Mode string
	// Paranoia 偏执等级 1-4，越高越严（误报也越多）。
	Paranoia int
}

// 只保留请求阶段（911xxx-944xxx）的检测规则，过滤初始化/评分/关联这类噪声规则。
const (
	crsMinRuleID = 910000
	crsMaxRuleID = 949000
	crsMaxScore  = 300

	// 协议合规类规则的号段与降权后的权重。
	//
	// 这批规则检查的是 HTTP 报文规范（Content-Type 缺失/重复、Host 写成 IP、
	// 请求行不规范…），而真实业务里大量 API 客户端、老 SDK、反向代理会发出
	// "不规范但无害"的报文。它们在 CRS 里标成 CRITICAL，按 60 分换算正好等于
	// 默认拦截阈值——线上实测正常登录 POST 被 920620 判成 60 分直接 403。
	//
	// 因此协议类命中只作证据：单条 15 分，总和封顶 29，永不单独越过默认校验
	// 阈值 30。要拦截必须配合真正的内容类规则（SQLi/XSS/RCE 等）。
	//
	// 覆盖 920xxx（协议强制）、921xxx（请求走私）与 922xxx（multipart 合规，
	// v4.30 新增：检查 part 的 charset / content-type 写法）：三者同样依赖
	// 报文写法，正常 API 客户端也会被误判，一并降权。
	crsProtocolMin    = 920000
	crsProtocolMax    = 923000
	crsProtocolWeight = 15
	crsProtocolCap    = 29
)

// isProtocolRule 判断是否协议合规类规则。
func isProtocolRule(id int) bool {
	return id >= crsProtocolMin && id < crsProtocolMax
}

// NewCRSEngine 加载内嵌的 OWASP CRS。加载耗时约 70ms，只在 Reload 时重建。
func NewCRSEngine(opt CRSOptions) (*CRSEngine, error) {
	mode := "block"
	if strings.EqualFold(opt.Mode, "log") {
		mode = "log"
	}
	pl := opt.Paranoia
	if pl < 1 || pl > 4 {
		pl = 1
	}
	engineMode := "On"
	if mode == "log" {
		engineMode = "DetectionOnly"
	}

	// 顺序很重要：偏执等级必须在 901-INITIALIZATION 之前写入 TX，
	// 否则 CRS 会先按默认 PL1 初始化，后面的设置不生效。
	directives := fmt.Sprintf(`
SecRuleEngine %s
SecAction "id:900900,phase:1,pass,nolog,setvar:tx.blocking_paranoia_level=%d,setvar:tx.detection_paranoia_level=%d"
Include @coraza.conf-recommended
Include @crs-setup.conf.example
Include @owasp_crs/*.conf
`, engineMode, pl, pl)

	// crs/v4 提供 @owasp_crs/ 与 @crs-setup.conf.example（内嵌 CRS v4.30.0），
	// coraza/v3 提供 @coraza.conf-recommended，两者用 mergefs 合并成根 FS；
	// crspatch 补 v4.30.0 打包遗漏的 web-shells-php.data（排第一，优先级最高）。
	waf, err := coraza.NewWAF(coraza.NewWAFConfig().
		WithRootFS(mergefs.Merge(crspatch.FS, crsv4.FS, corazaconf.FS)).
		WithDirectives(directives))
	if err != nil {
		return nil, fmt.Errorf("加载 CRS 失败: %w", err)
	}
	return &CRSEngine{waf: waf, mode: mode, paranoia: pl, version: "crs-" + crsv4.Version}, nil
}

// Mode 返回生效模式。
func (c *CRSEngine) Mode() string {
	if c == nil {
		return ""
	}
	return c.mode
}

// Paranoia 返回偏执等级。
func (c *CRSEngine) Paranoia() int {
	if c == nil {
		return 0
	}
	return c.paranoia
}

// Version 返回规则集版本。
func (c *CRSEngine) Version() string {
	if c == nil {
		return ""
	}
	return c.version
}

// Inspect 跑一遍 CRS。返回的 Result 只填 Score/Hits/RuleIDs/Categories，
// 动作由调用方与自研引擎合并后再决定。
//
// 引擎内部异常一律降级为「无命中」，绝不能因为规则引擎把请求吞掉。
func (c *CRSEngine) Inspect(in *Input) (res Result) {
	res = Result{Hits: []Hit{}, Categories: []string{}, RuleIDs: []string{}}
	if c == nil || c.waf == nil || in == nil {
		return res
	}
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			res = Result{Hits: []Hit{}, Categories: []string{}, RuleIDs: []string{}}
		}
		res.ElapsedUS = time.Since(start).Microseconds()
	}()

	tx := c.waf.NewTransaction()
	defer tx.Close()

	uri := in.Path
	if in.Query != "" {
		uri += "?" + in.Query
	}
	method := in.Method
	if method == "" {
		method = "GET"
	}
	if in.Host != "" {
		tx.SetServerName(in.Host)
	}
	tx.ProcessConnection("", 0, "", 0)
	tx.ProcessURI(uri, method, "HTTP/1.1")

	for _, kv := range splitHeaderLines(in.Headers) {
		if i := strings.Index(kv, ":"); i > 0 {
			tx.AddRequestHeader(strings.TrimSpace(kv[:i]), strings.TrimSpace(kv[i+1:]))
		}
	}
	if in.Host != "" {
		tx.AddRequestHeader("Host", in.Host)
	}
	if in.UA != "" {
		tx.AddRequestHeader("User-Agent", in.UA)
	}
	if in.Referer != "" {
		tx.AddRequestHeader("Referer", in.Referer)
	}
	if in.Cookies != "" {
		tx.AddRequestHeader("Cookie", in.Cookies)
	}
	if in.Body != "" {
		tx.AddRequestHeader("Content-Type", "application/x-www-form-urlencoded")
	}

	if it := tx.ProcessRequestHeaders(); it != nil {
		res.Score = 100
	}
	// 即使没有 body 也必须走 ProcessRequestBody：CRS 的 942xxx（SQLi）、
	// 930xxx（LFI）等检测规则注册在 phase 2，不进这个阶段永远不会执行。
	if in.Body != "" {
		if w, ok := tx.(interface {
			WriteRequestBody([]byte) (*types.Interruption, int, error)
		}); ok {
			_, _, _ = w.WriteRequestBody([]byte(in.Body))
		}
	}
	_, _ = tx.ProcessRequestBody()

	seen := map[int]struct{}{}
	score := 0
	protoScore := 0
	for _, mr := range tx.MatchedRules() {
		rule := mr.Rule()
		id := rule.ID()
		if id < crsMinRuleID || id >= crsMaxRuleID {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		weight := severityWeight(rule.Severity())
		if weight == 0 {
			continue
		}
		// 协议合规类（920xxx）单独降权并设总上限，避免"报文不规范"被判成攻击。
		// 详见 isProtocolRule 的注释。
		if isProtocolRule(id) {
			if weight > crsProtocolWeight {
				weight = crsProtocolWeight
			}
			if protoScore >= crsProtocolCap {
				weight = 0
			} else if protoScore+weight > crsProtocolCap {
				weight = crsProtocolCap - protoScore
			}
			protoScore += weight
		}
		score += weight
		cat := crsCategory(id)
		evidence := mr.Data()
		if evidence == "" {
			if ds := mr.MatchedDatas(); len(ds) > 0 {
				evidence = ds[0].Value()
			}
		}
		if len(evidence) > 120 {
			evidence = evidence[:120]
		}
		res.Hits = append(res.Hits, Hit{
			RuleID:   fmt.Sprintf("crs-%d", id),
			Name:     mr.Message(),
			Category: cat,
			Score:    weight,
			Target:   "crs",
			Evidence: evidence,
		})
		res.RuleIDs = append(res.RuleIDs, fmt.Sprintf("crs-%d", id))
		res.Categories = appendUnique(res.Categories, cat)
	}
	if score > crsMaxScore {
		score = crsMaxScore
	}
	res.Score = score

	// CRS 自己的拦截阈值（默认 5 分）已由 949110 判定；On 模式下拿到 interruption
	// 就说明确实过线，直接按截断处理（分数拉满），不依赖我们的阈值换算。
	if it := tx.Interruption(); it != nil {
		res.Score = crsMaxScore
		res.Action = "block"
	}
	sort.Strings(res.RuleIDs)
	return res
}

// severityWeight 把 CRS 严重级别折算成我们的分值。
//
// 对齐关系：一条 critical = 60 分 = 默认 block 阈值，即「单条严重规则即拦」；
// 与「宁可错杀」的策略一致。
func severityWeight(s types.RuleSeverity) int {
	switch s {
	case types.RuleSeverityEmergency:
		return 100
	case types.RuleSeverityAlert:
		return 80
	case types.RuleSeverityCritical:
		return 60
	case types.RuleSeverityError:
		return 45
	case types.RuleSeverityWarning:
		return 30
	case types.RuleSeverityNotice:
		return 15
	default:
		return 0
	}
}

// crsCategory 按 CRS 的规则号段映射到面板分类。
func crsCategory(id int) string {
	switch {
	case id >= 913000 && id < 914000, id >= 911000 && id < 913000:
		return "scanner"
	case id >= 920000 && id < 930000:
		return "protocol"
	case id >= 930000 && id < 931000:
		return "traversal" // LFI
	case id >= 931000 && id < 932000:
		return "rce" // RFI
	case id >= 932000 && id < 933000:
		return "rce"
	case id >= 933000 && id < 934000:
		return "rce" // PHP 注入
	case id >= 934000 && id < 935000:
		return "rce" // 通用/SSRF
	case id >= 941000 && id < 942000:
		return "xss"
	case id >= 942000 && id < 943000:
		return "sqli"
	case id >= 943000 && id < 944000:
		return "protocol"
	case id >= 944000 && id < 945000:
		return "rce"
	default:
		return "protocol"
	}
}

// splitHeaderLines 把 "K: v K2: v2" 形式的头串拆开。
func splitHeaderLines(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0, 8)
	for _, part := range strings.Split(s, " ") {
		if strings.Contains(part, ":") {
			out = append(out, part)
		} else if len(out) > 0 {
			// 头值里带空格的情况，拼回上一条
			out[len(out)-1] += " " + part
		}
	}
	return out
}
