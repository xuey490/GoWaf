package detect

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
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
	waf      coraza.WAF       // Coraza 引擎实例，构建后不可变、可并发使用
	mode     string           // block / log
	paranoia int              // 偏执等级 1-4
	methods  string           // 911100 方法白名单
	version  string           // 规则集版本，形如 "crs-4.30.0"
	packages int              // 编译进来的 SecLang 规则包数量
	pkgIDs   map[int]struct{} // 规则包自定义 rule id 集合，供命中过滤放行
}

// CRSOptions 构造参数。
type CRSOptions struct {
	// Mode: block（SecRuleEngine On，命中阈值即拦截）/ log（DetectionOnly，只记录）
	Mode string
	// Paranoia 偏执等级 1-4，越高越严（误报也越多）。
	Paranoia int
	// AllowedMethods 911100 方法白名单。CRS 默认只放 GET HEAD POST OPTIONS，
	// 前后端分离的 REST API（PUT/DELETE/PATCH）全部命中 911100（CRITICAL 60 分）
	// 直接 403。这里在 901-INITIALIZATION 之前写入 TX 覆盖默认值；空串用默认集。
	AllowedMethods string
	// Packages 追加编译进引擎的 SecLang 规则包（按导入顺序 Include）。
	// 调用方负责先逐包 ValidateSecLang，编译失败的包不要传进来。
	Packages []CRSPackage
}

// DefaultCRSMethods 默认方法白名单：REST API 常规方法集。
// 若站点纯浏览器表单业务，可在设置里收紧为 "GET HEAD POST OPTIONS"。
const DefaultCRSMethods = "GET HEAD POST OPTIONS PUT DELETE PATCH"

// 只保留请求阶段（911xxx-944xxx）的检测规则，过滤初始化/评分/关联这类噪声规则。
const (
	crsMinRuleID = 910000 // 参与评分的 CRS 请求阶段规则起始号段（含）
	crsMaxRuleID = 949000 // 结束号段（不含）
	crsMaxScore  = 300    // CRS 引擎单次得分的总上限

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
// 参数 id：CRS 规则号。返回 bool：是否落在 920000-923000 号段（需降权）。
func isProtocolRule(id int) bool {
	return id >= crsProtocolMin && id < crsProtocolMax
}

// NewCRSEngine 加载内嵌的 OWASP CRS。加载耗时约 70ms，只在 Reload 时重建。
// 参数 opt：模式、偏执等级、方法白名单与追加编译的 SecLang 规则包。
// 返回 *CRSEngine：可并发使用的 CRS 引擎；返回 error：Coraza 编译指令串失败时。
func NewCRSEngine(opt CRSOptions) (*CRSEngine, error) {
	mode := "block"
	if strings.EqualFold(opt.Mode, "log") {
		mode = "log"
	}
	pl := opt.Paranoia
	if pl < 1 || pl > 4 {
		pl = 1 // 越界回退默认 PL1
	}
	engineMode := "On"
	if mode == "log" {
		engineMode = "DetectionOnly"
	}

	// 顺序很重要：偏执等级必须在 901-INITIALIZATION 之前写入 TX，
	// 否则 CRS 会先按默认 PL1 初始化，后面的设置不生效。
	// allowed_methods 同理：901-INITIALIZATION 只在未设置时才写默认值
	// （GET HEAD POST OPTIONS），先写才能覆盖；不然前后端分离的
	// PUT/DELETE/PATCH 会被 911100 判 CRITICAL 直接拦掉。
	methods := sanitizeMethods(opt.AllowedMethods)
	var sb strings.Builder
	fmt.Fprintf(&sb, `
SecRuleEngine %s
SecAction "id:900900,phase:1,pass,nolog,setvar:tx.blocking_paranoia_level=%d,setvar:tx.detection_paranoia_level=%d,setvar:'tx.allowed_methods=%s'"
Include @coraza.conf-recommended
Include @crs-setup.conf.example
Include @owasp_crs/*.conf
`, engineMode, pl, pl, methods)
	// SecLang 规则包排在 CRS 之后：CRS 的 901/949/959 评分框架先就位，
	// 包里的排除规则（RemoveById）和自定义检测才能按预期叠加。
	// 注意：内容直接内联而非 Include——Coraza 对相对路径 Include 读的是
	// 进程工作目录而非 RootFS，虚拟 FS 里的文件解析不到。
	for _, p := range opt.Packages {
		sb.WriteString("\n# seclang package: " + p.ID + "\n")
		sb.WriteString(strings.TrimRight(p.Content, "\n"))
		sb.WriteString("\n")
	}

	// 登记规则包里的 rule id：Inspect 的命中过滤只认 CRS 号段
	// （910000-949000），自定义包的 id（1-99999 / 100000+）必须单独放行，
	// 否则包规则只拦不记，面板上看不到命中。
	pkgIDs := collectPkgRuleIDs(opt.Packages)

	// crs/v4 提供 @owasp_crs/ 与 @crs-setup.conf.example（内嵌 CRS v4.30.0），
	// coraza/v3 提供 @coraza.conf-recommended，两者用 mergefs 合并成根 FS；
	// crspatch 补 v4.30.0 打包遗漏的 web-shells-php.data（排第一，优先级最高）。
	rootFS := mergefs.Merge(crspatch.FS, crsv4.FS, corazaconf.FS)

	waf, err := coraza.NewWAF(coraza.NewWAFConfig().
		WithRootFS(rootFS).
		WithDirectives(sb.String()))
	if err != nil {
		return nil, fmt.Errorf("加载 CRS 失败: %w", err)
	}
	return &CRSEngine{waf: waf, mode: mode, paranoia: pl, methods: methods, version: "crs-" + crsv4.Version,
		packages: len(opt.Packages), pkgIDs: pkgIDs}, nil
}

// sanitizeMethods 清洗方法白名单：只留大写字母与空格，防止注入破坏指令串。
// 参数 v：用户配置的原始方法串。返回 string：清洗后的串；为空时回退默认集。
func sanitizeMethods(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v == "" {
		return DefaultCRSMethods
	}
	var b strings.Builder
	for _, r := range v {
		if (r >= 'A' && r <= 'Z') || r == ' ' {
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if out == "" {
		return DefaultCRSMethods
	}
	return out
}

// Methods 返回当前方法白名单（用于 reload 缓存比对）。
// 无参数；接收者为 nil 时返回空串。
func (c *CRSEngine) Methods() string {
	if c == nil {
		return ""
	}
	return c.methods
}

// pkgIDRe 从 SecLang 文本里粗提取 rule id（id:123 / id: 123 / id:'123'）。
var pkgIDRe = regexp.MustCompile(`\bid\s*[:=]\s*'?(\d{1,7})`)

// collectPkgRuleIDs 汇总所有包定义的 rule id。
// 参数 pkgs：SecLang 规则包列表。返回 map[int]struct{}：包内出现的 rule id 集合。
// 解析失败就漏登记，后果只是该规则不进命中列表，不影响拦截（interruption 仍生效）。
func collectPkgRuleIDs(pkgs []CRSPackage) map[int]struct{} {
	ids := map[int]struct{}{}
	for _, p := range pkgs {
		for _, m := range pkgIDRe.FindAllStringSubmatch(p.Content, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				ids[n] = struct{}{}
			}
		}
	}
	return ids
}

// Packages 返回编译进当前引擎的规则包数量。
// 无参数；接收者为 nil 时返回 0。
func (c *CRSEngine) Packages() int {
	if c == nil {
		return 0
	}
	return c.packages
}

// Mode 返回生效模式。
// 无参数；接收者为 nil 时返回空串。
func (c *CRSEngine) Mode() string {
	if c == nil {
		return ""
	}
	return c.mode
}

// Paranoia 返回偏执等级。
// 无参数；接收者为 nil 时返回 0。
func (c *CRSEngine) Paranoia() int {
	if c == nil {
		return 0
	}
	return c.paranoia
}

// Version 返回规则集版本。
// 无参数；接收者为 nil 时返回空串。
func (c *CRSEngine) Version() string {
	if c == nil {
		return ""
	}
	return c.version
}

// Inspect 跑一遍 CRS。返回的 Result 只填 Score/Hits/RuleIDs/Categories，
// 动作由调用方与自研引擎合并后再决定。
// 参数 in：待检测的请求数据。返回 Result：CRS 命中的得分与明细（含耗时统计）。
//
// 引擎内部异常一律降级为「无命中」，绝不能因为规则引擎把请求吞掉。
func (c *CRSEngine) Inspect(in *Input) (res Result) {
	res = Result{Hits: []Hit{}, Categories: []string{}, RuleIDs: []string{}}
	if c == nil || c.waf == nil || in == nil {
		return res
	}
	start := time.Now()
	// panic 兜底：引擎内部异常时返回空结果，仅保留耗时统计
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

	// 把头串逐条喂给事务，驱动依赖请求头的 phase 1 规则
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
		// 有请求体时补表单类型 Content-Type，让 phase 2 能解析出 body 参数
		tx.AddRequestHeader("Content-Type", "application/x-www-form-urlencoded")
	}

	// 头阶段即被中断说明命中了拦截型规则；此处先记高分，最终得分以文末统计为准
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

	seen := map[int]struct{}{} // 按 rule id 去重：同一规则的多条匹配只记一次
	score := 0
	protoScore := 0
	for _, mr := range tx.MatchedRules() {
		rule := mr.Rule()
		id := rule.ID()
		// 只统计 CRS 号段内的规则；号段外若属规则包自定义 id 也放行
		_, isPkg := c.pkgIDs[id]
		if id < crsMinRuleID || id >= crsMaxRuleID {
			if !isPkg {
				continue
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		weight := severityWeight(rule.Severity())
		if weight == 0 && isPkg && (id < crsMinRuleID || id >= crsMaxRuleID) {
			// 包规则没标 severity 时给默认 40 分，保证能进评分。
			weight = 40
		}
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
		prefix := "crs"
		if isPkg && (id < crsMinRuleID || id >= crsMaxRuleID) {
			// 规则包自定义 id 不在 CRS 号段：单独归类，便于面板区分来源。
			cat = "seclang"
			prefix = "seclang"
		}
		// 证据优先取匹配数据，缺失时回退第一条匹配变量；统一截断到 120 字节
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
			RuleID:   fmt.Sprintf("%s-%d", prefix, id),
			Name:     mr.Message(),
			Category: cat,
			Score:    weight,
			Target:   "crs",
			Evidence: evidence,
		})
		res.RuleIDs = append(res.RuleIDs, fmt.Sprintf("%s-%d", prefix, id))
		res.Categories = appendUnique(res.Categories, cat)
	}
	// 总分封顶，防止单请求刷出极端分值
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
// 参数 s：CRS 规则严重级别。返回 int：对应分值，未识别级别返回 0（不计分）。
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
// 参数 id：CRS 规则号。返回 string：分类名；未覆盖的号段归入 protocol。
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
// 参数 s：空格分隔的头串。返回 []string：逐条 "K: v" 头；值里带空格的会拼回上一条。
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
