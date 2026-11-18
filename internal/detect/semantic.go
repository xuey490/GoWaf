package detect

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// SemanticEngine 是第四检测通道：语义/结构判定，覆盖 SQLi/XSS（libinjection）之外的
// 命令注入、代码执行、XXE、SSRF、SSTI、表达式注入与反序列化七大类。
//
// 为什么不靠规则表：这批攻击的变形靠"关键词+正则"永远追不完（雷池语义引擎的
// 核心壁垒即在于此）。本通道用"结构特征"判定——例如命令注入要求"元字符紧邻
// 系统命令"，命令注入的判定单元是"结构"而非具体关键词，攻击者换命令、换编码
// （Normalize 已做多轮 URL 解码 / HTML 实体还原）都绕不开结构。
//
// 与 CRS 的分工：CRS 覆盖广但以特征为主且成本高；本通道是轻量叠加证据——
// 单类命中分数压在默认拦截阈值之下（最高 50 < block 60），多类并发或与
// CRS/自研规则叠加才跨过拦截线，误报的代价是一次无感人机校验而非直接 403。
//
// 误报治理：与其他引擎共用例外规则体系（RuleID 带 sem- 前缀、Category 独立，
// 面板可按 ID 精确豁免）；扫描部位与 libi 一致只扫 query/body 及解码变体，
// 唯独表达式注入额外扫 header（Log4Shell 经 UA/Referer 注入是经典路径）。
type SemanticEngine struct {
	enabled atomic.Bool
	scores  atomic.Pointer[semScores]
}

// semScores 各类别的命中加分快照（Reload 时整体替换）。
type semScores struct {
	cmdi, rce, ssrf, xxe, ssti, expr, deser int
}

// 语义通道默认分值：均在默认挑战阈值(30)之上、拦截阈值(60)之下，
// 单类命中触发人机校验，叠加或更高危类别由全局评分体系决定拦截。
const (
	DefaultSemCmdiScore  = 45
	DefaultSemRCEScore   = 50
	DefaultSemSSRFScore  = 35
	DefaultSemXXEScore   = 40
	DefaultSemSSTIScore  = 40
	DefaultSemExprScore  = 45
	DefaultSemDeserScore = 40
)

// 语义通道规则标识与分类（面板/例外规则按 ID 精确治理）。
const (
	RuleIDSemCmdi  = "sem-cmdi"
	RuleIDSemRCE   = "sem-rce"
	RuleIDSemSSRF  = "sem-ssrf"
	RuleIDSemXXE   = "sem-xxe"
	RuleIDSemSSTI  = "sem-ssti"
	RuleIDSemExpr  = "sem-expr"
	RuleIDSemDeser = "sem-deser"

	CategoryCmdi  = "cmdi"
	CategoryRCE   = "rce"
	CategorySSRF  = "ssrf"
	CategoryXXE   = "xxe"
	CategorySSTI  = "ssti"
	CategoryExpr  = "expr"
	CategoryDeser = "deser"
)

// semScanLimit 单部位扫描文本上限（字节）。与 libi 同理：32KB 之后的
// 攻击 payload 几乎不存在，超大 body 全量跑正则纯浪费。
const semScanLimit = 32 * 1024

// semRule 一条结构判定式。onRaw 为 true 时匹配未压缩原文（保留换行等原始
// 空白），用于只对原文成立的特征（如换行注入）；false 时匹配 Normalize
// 归一化文本（URL 解码、HTML 实体还原、全角转半角、小写化）。
type semRule struct {
	re    *regexp.Regexp
	onRaw bool
}

// mustRe 编译辅助：语义规则在编译期写死，正则非法属于程序 bug，直接 panic 暴露。
func mustRe(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(fmt.Sprintf("sem 规则编译失败 %q: %v", pattern, err))
	}
	return re
}

// semCatRule 各类别的结构判定式。
//
// 正则书写约束（Go RE2 不支持回溯与前瞻，全部用字符类表达）：
//   - 元字符紧邻要求：命令注入一律要求 ;|`$(&& 之一紧邻系统命令，单 & / 单 |
//     必须再跟参数（避免 URL 参数分隔符 & 造成海量误报）；
//   - 模糊命令词（cat/ls/id 等常见英文词）后必须跟空白或引号，明确词不要求；
//   - SSRF 只标记非 http 的危险 scheme 或"scheme+内网地址"组合，裸域名不判。
//
// real sem rule sets —— 为可读性把每类拆成命名组。
var (
	// semCmdiRules 命令注入：元字符 + 系统命令 的结构组合。
	// 尾部分隔符类统一为 [\s),;&|'"] 或行尾（$）：命令词后跟 = 的参数形态不判。
	semCmdiRules = []semRule{
		// 高置信系统词：; | && ` $( 紧邻即判
		{re: mustRe(`[;&|]\s*(whoami|uname|ncat?\b|nc -|telnet|wget|curl|chmod|chown|base64|xxd|strings|nslookup|dig |ifconfig|ipconfig|netstat|apt(?:-get)? |yum |pip[23]? |npm )`)},
		// 模糊词：要求 ; 前缀（查询串里 & 太常见、| 少见但仍有 a=1|cat 风险）
		{re: mustRe(`;\s*(cat|ls|id|pwd|head|tail|sleep|ping|rm |cp |mv |find|grep|awk|sed|touch|kill|ps )`)},
		// 管道/&&/|| + 常见命令词，要求其后跟空白或引号或结尾（排除 |cat=5 形式）
		{re: mustRe(`\|\s*(cat|ls|id|pwd|head|tail|sleep|ping|wget|curl|nc|bash|zsh|sh|python[23]?|perl|php|ruby)([\s;|&'"]|$)`)},
		{re: mustRe(`&&\s*(cat|ls|id|pwd|head|tail|sleep|ping|wget|curl|nc|bash|zsh|sh|python[23]?|perl|php|ruby)([\s;|&'"]|$)`)},
		// 命令替换 $() 与反引号内的高置信命令
		{re: mustRe(`\$\(\s*(whoami|uname|cat|ls|id|pwd|wget|curl|nc|bash|sh|zsh|ping|sleep|head|tail|find|grep)([\s),;&|'"]|$)`)},
		{re: mustRe("`\\s*(whoami|uname|cat|ls|id|pwd|wget|curl|nc|bash|sh|ping|sleep)([\\s),;&|'\"`]|$)")},
		// 经典读文件：cat/less/more/tac + /etc/ 或 /proc/
		{re: mustRe(`\b(cat|head|tail|more|less|tac)\s+/(etc|proc|root|home)/[^=\s]{2,}`)},
		// 管道直执行：curl/wget 下载后 |sh —— 供应链攻击的经典链路
		{re: mustRe(`(wget|curl)\s+https?://[^\s|]{4,}\s*\|\s*(sudo\s+)?(ba|z|da|k)?sh\b`)},
		// 换行注入：仅对原文匹配（Normalize 会把换行压成空格）
		{re: mustRe(`[\r\n]\s*(whoami|uname|cat\s|id\s|pwd\s|wget\s|curl\s|bash|nc\s|sleep\s|ping\s)`), onRaw: true},
		// URL 编码的换行注入：%0a/%0d 在原文里就是字面量
		{re: mustRe(`%0[ad]\s*(whoami|uname|cat |id |pwd |wget |curl |bash|nc |sleep |ping )`), onRaw: true},
	}

	// semRCERules 远程代码执行：各语言 eval/system 结构。
	semRCERules = []semRule{
		// PHP：高危函数（system/passthru/shell_exec 不与普通词冲突）；eval 要求危险参数
		{re: mustRe(`\b(system|passthru|shell_exec|proc_open|pcntl_exec)\s*\(`)},
		{re: mustRe(`\beval\s*\(\s*(base64_decode|gzinflate|gzuncompress|str_rot13|rawurldecode|\$_(get|post|request|cookie))`)},
		{re: mustRe(`\bassert\s*\(\s*\$_(get|post|request)`)},
		// PHP 伪协议：php://input / php://filter（读源码经典）、data://、expect://
		{re: mustRe(`php://(?:input|filter|expect|fd)|data://text|expect://|zip://|phar://`)},
		// Python：动态导入与系统调用
		{re: mustRe(`__import__\s*\(|os\.(system|popen)\s*\(|subprocess\.(call|run|Popen|check_output)\s*\(`)},
		{re: mustRe(`\bexec\s*\(\s*(__import__|input|open|compile)\s*\(`)},
		// Java：Runtime.exec / ProcessBuilder / JNDI InitialContext 注入
		{re: mustRe(`runtime\.getruntime\s*\(\s*\)\s*\.\s*exec|new\s+processbuilder|initialcontext\s*\(\s*\)\s*\.\s*lookup`)},
		// JSP 一句话/服务端模板执行
		{re: mustRe(`\.exec\s*\(\s*["'][^"']*(cmd|bash|sh)\b`)},
		// ASP/Windows：wscript.shell / cmd.exe 调用
		{re: mustRe(`wscript\.shell|createobject\s*\(\s*["']?wscript|cmd\.exe\s+/c`)},
		// Node.js：child_process 执行
		{re: mustRe(`child_process|\.exec\s*\(\s*req\.(query|body|params)`)},
	}

	// semXXERules XXE：DOCTYPE + ENTITY 的文档结构。
	semXXERules = []semRule{
		// DOCTYPE 与 ENTITY 同时出现（文档型特征，注释/文本里几乎不会成对出现）
		{re: mustRe(`<!doctype[^>]{0,200}>[\s\S]{0,400}?<!entity`)},
		// 外部 DTD：DOCTYPE 直接 SYSTEM 指向外部地址
		{re: mustRe(`<!doctype[^>]{0,200}system\s+["']?(https?|file|ftp|gopher|expect)://`)},
		// 外部实体：SYSTEM + 危险协议
		{re: mustRe(`<!entity\s+\w+\s+(system|public)\s+["']?(file|php|expect|gopher|dict|ftp|http)://`)},
		{re: mustRe(`<!entity\s+\w+\s+system\s+["']?/`)},
		// 参数实体 + SYSTEM（盲 XXE）
		{re: mustRe(`<!entity\s+%\s+\w+\s+system`)},
	}

	// semSSRFRules SSRF：危险 scheme 或 scheme+内网地址组合。
	semSSRFRules = []semRule{
		// 高危协议：gopher（可打 Redis/MySQL）、dict、file、tftp、sftp
		{re: mustRe(`(gopher|dict|tftp|sftp|ldap[s]?)://[^\s"']{3,}`)},
		// file:// + Unix 路径
		{re: mustRe(`file:///(etc|proc|root|home|var|tmp|usr)/`)},
		// http(s) 指向回环 / 内网保留段 / 云元数据端点
		{re: mustRe(`https?://(127\.0\.0\.1|\[::1\]|localhost|0\.0\.0\.0|0177\.0\.0\.1|2130706433)([:/?"']|$)`)},
		{re: mustRe(`169\.254\.169\.254|metadata\.google\.internal|100\.100\.100\.200`)},
		// 十进制/八进制 IP 变体指向内网（http://2130706433 已在上面；此处补独立形态）
		{re: mustRe(`https?://0x7f\.0\.0\.1|https?://0x7f000001`)},
	}

	// semSSTIRules 服务端模板注入：模板语法内的运算/属性链。
	semSSTIRules = []semRule{
		// Jinja/twig：{{expr}} 内含运算或 Python 魔法属性
		{re: mustRe(`\{\{\s*[^{}]{0,80}?[*+][^{}]{0,80}?\}\}`)},
		{re: mustRe(`\{\{[^{}]{0,120}?(__class__|__mro__|__subclasses__|__globals__|__import__|\.popen|lipsum|cycler|joiner|namespace)[^{}]{0,120}?\}\}`)},
		// Jinja 语句块 {% ... %} 带危险调用
		{re: mustRe(`\{%[^%]{0,120}?(import|popen|config|self\.|os\.|open\()[^%]{0,120}?%\}`)},
		// ERB/EJS：<%= ... %> 内含系统调用或 Ruby 对象遍历
		{re: mustRe(`<%=\s*[^%]{0,120}?(system\s*\(|io\.popen|kernel#|instance_variable_get)[^%]{0,120}?%>`)},
		// Velocity：#set 与反射执行（getClass().forName 链）
		{re: mustRe(`#set\s*\(\s*\$\w+\s*=\s*[^)]{0,80}(getclass|newinstance|runtime)`)},
		{re: mustRe(`getclass\s*\(\s*\)\s*\.\s*(forname|newinstance)|\.forname\s*\(\s*["']java\.lang\.runtime`)},
		// FreeMarker：<#assign +危险内建
		{re: mustRe(`<#assign\s+\w+\s*=[^>]{0,120}(\.exec|objectwrapper|"freemarker\.template\.utility\.Execute)`)},
		// Twig：{{...}} 内 filter 链 + map/system
		{re: mustRe(`\{\{[^{}]{0,80}\|\s*(map|filter|sort|reduce)\s*\(?\s*[^{}]{0,80}(system|exec|passthru|shell_exec)`)},
	}

	// semExprRules 表达式/语言注入：SpEL、OGNL、EL、JNDI（Log4Shell）。
	semExprRules = []semRule{
		// JNDI lookup：Log4Shell 及各类 ${jndi:*} 注入（UA/Referer/路径均可携带）
		{re: mustRe(`\$\{jndi:(ldap[s]?|rmi|dns|iiop|corba|http|nds):`)},
		// ${jndi:${lower:...}} 等嵌套混淆变体
		{re: mustRe(`\$\{jndi:\s*\$\{`)},
		// OGNL：@java.lang.Runtime@getRuntime / %{...} 内反射
		{re: mustRe(`@\s*java\.lang\.runtime@getruntime|%{[^}]{0,80}(@|runtime|processbuilder)[^}]{0,80}}`)},
		// SpEL：T(java.lang.Runtime) / new java.lang.ProcessBuilder
		{re: mustRe(`#\{[^}]{0,120}?t\s*\(\s*java\.lang|new\s+java\.lang\.processbuilder`)},
		// JSP EL：pageContext / servletContext 属性链
		{re: mustRe(`\$\{pagecontext\.(request|servletcontext)|\$\{servletcontext\.`)},
		// Struts2 OGNL 载荷经典片段
		{re: mustRe(`(#context\[|#_memberaccess|ognl\.expression)`)},
		// Spring 表达式 ref 前缀（SpEL 注入入口）
		{re: mustRe(`\$\{[^}]{0,60}?t\(java\.lang\.runtime\)`)},
	}

	// semDeserRules 反序列化：各语言序列化格式的魔数特征。
	// 注意：全部按小写书写——检测文本经 Normalize 已统一小写化。
	semDeserRules = []semRule{
		// Java 序列化：base64 头 rO0AB（AC ED 00 05）与 hex 原文
		{re: mustRe(`ro0ab[a-z0-9+/]{8,}`)},
		{re: mustRe(`aced0005|\xac\xed\x00\x05`)},
		// .NET BinaryFormatter：AAEAAAD/////（00 01 00 00 00 FF FF FF FF）
		{re: mustRe(`aaeaaad/+`)},
		// Python pickle：base64 头 gASV（\x80\x04\x95）与协议操作码
		{re: mustRe(`gasv[a-z0-9+/]{8,}|cos\nsystem|\x80\x04\x95`)},
		// YAML 反序列化：!!python/ 标签（/ 与 : 均可跟后续路径）
		{re: mustRe(`!!python/(object|apply|name|module)[/\s:]`)},
		// PHP 反序列化：O:类名长度:" 类对象结构（a: 数组太常见不判）
		{re: mustRe(`o:\d+:"`)},
		// ASP.NET ViewState：/wE 前缀的长 base64（加密视图状态的特征头）或超长 __VIEWSTATE
		{re: mustRe(`/we[a-z0-9+/]{16,}|__viewstate=[a-z0-9+/]{200,}`)},
	}
)

// semRulesByCat 汇总各类别的判定式（引用上面的命名组）。
func semRulesByCat() map[string][]semRule {
	return map[string][]semRule{
		CategoryCmdi:  semCmdiRules,
		CategoryRCE:   semRCERules,
		CategoryXXE:   semXXERules,
		CategorySSRF:  semSSRFRules,
		CategorySSTI:  semSSTIRules,
		CategoryExpr:  semExprRules,
		CategoryDeser: semDeserRules,
	}
}

// semCatName 类别 → 中文名（事件展示用）。
var semCatName = map[string]string{
	CategoryCmdi:  "命令注入",
	CategoryRCE:   "远程代码执行",
	CategoryXXE:   "XXE 外部实体注入",
	CategorySSRF:  "SSRF 服务端请求伪造",
	CategorySSTI:  "SSTI 模板注入",
	CategoryExpr:  "表达式/语言注入",
	CategoryDeser: "反序列化载荷",
}

// semCatRuleID 类别 → 规则 ID。
var semCatRuleID = map[string]string{
	CategoryCmdi:  RuleIDSemCmdi,
	CategoryRCE:   RuleIDSemRCE,
	CategoryXXE:   RuleIDSemXXE,
	CategorySSRF:  RuleIDSemSSRF,
	CategorySSTI:  RuleIDSemSSTI,
	CategoryExpr:  RuleIDSemExpr,
	CategoryDeser: RuleIDSemDeser,
}

// NewSemanticEngine 创建语义检测通道。
// 参数 enabled：是否启用；scores：类别 → 加分映射（缺失或 <=0 的类别回退默认值）。
// 返回 *SemanticEngine：空输入引擎实例，等待 ReloadSettings 装配参数。
func NewSemanticEngine(enabled bool, scores map[string]int) *SemanticEngine {
	e := &SemanticEngine{}
	e.enabled.Store(enabled)
	e.setScores(scores)
	return e
}

// setScores 装配分值快照（内部方法）。
func (e *SemanticEngine) setScores(scores map[string]int) {
	s := &semScores{
		cmdi:  DefaultSemCmdiScore,
		rce:   DefaultSemRCEScore,
		ssrf:  DefaultSemSSRFScore,
		xxe:   DefaultSemXXEScore,
		ssti:  DefaultSemSSTIScore,
		expr:  DefaultSemExprScore,
		deser: DefaultSemDeserScore,
	}
	if scores != nil {
		if v := scores[CategoryCmdi]; v > 0 {
			s.cmdi = v
		}
		if v := scores[CategoryRCE]; v > 0 {
			s.rce = v
		}
		if v := scores[CategorySSRF]; v > 0 {
			s.ssrf = v
		}
		if v := scores[CategoryXXE]; v > 0 {
			s.xxe = v
		}
		if v := scores[CategorySSTI]; v > 0 {
			s.ssti = v
		}
		if v := scores[CategoryExpr]; v > 0 {
			s.expr = v
		}
		if v := scores[CategoryDeser]; v > 0 {
			s.deser = v
		}
	}
	e.scores.Store(s)
}

// Enabled 报告通道是否启用。
func (e *SemanticEngine) Enabled() bool { return e != nil && e.enabled.Load() }

// Inspect 对 Query/Body（含解码变体）跑七类语义判定，Header 只扫表达式注入。
// 参数 in：已标准化的请求数据（与自研引擎共用同一 Input）。
// 返回 Result：命中明细与分数；未启用或无命中时返回零分空结果。
//
// 与 libi 相同的取值纪律：每类在全部扫描文本中只记一次命中（分数不随出现
// 次数翻倍），误报治理走例外规则。
func (e *SemanticEngine) Inspect(in *Input) Result {
	start := time.Now()
	res := Result{Hits: []Hit{}, Categories: []string{}, RuleIDs: []string{}}
	if !e.Enabled() {
		return res
	}
	sc := e.scores.Load()
	if sc == nil {
		return res
	}

	// 组装扫描清单：部位 → 归一化文本列表。query/body 含解码变体；header
	// 只喂表达式注入（jndi 走 UA/Referer 是 Log4Shell 的经典路径）。
	type scanItem struct {
		target string
		text   string
		raw    string
	}
	var scans []scanItem
	addScan := func(target, raw string) {
		if raw == "" {
			return
		}
		n := Normalize(Truncate(raw, semScanLimit))
		if n != "" {
			scans = append(scans, scanItem{target: target, text: n, raw: Truncate(raw, semScanLimit)})
		}
	}
	addScan("query", in.Query)
	addScan("body", in.Body)
	for _, v := range in.DecodedQuery {
		addScan("query", v)
	}
	for _, v := range in.DecodedBody {
		addScan("body", v)
	}
	headerNorm := ""
	if in.Headers != "" || in.Referer != "" {
		headerNorm = Normalize(Truncate(in.Headers+" "+in.Referer, semScanLimit))
	}

	type catState struct {
		score int
		id    string
		seen  bool
	}
	cats := map[string]*catState{}
	markHit := func(cat, target, evidence string) {
		st := cats[cat]
		if st == nil {
			return
		}
		if st.seen {
			return
		}
		st.seen = true
		res.Score += st.score
		res.Hits = append(res.Hits, Hit{
			RuleID:   st.id,
			Name:     "语义识别" + semCatName[cat],
			Category: cat,
			Score:    st.score,
			Target:   target,
			Evidence: Truncate(evidence, 120),
		})
		res.RuleIDs = append(res.RuleIDs, st.id)
		res.Categories = appendUnique(res.Categories, cat)
	}
	for cat, st := range map[string]*catState{
		CategoryCmdi:  {score: sc.cmdi, id: RuleIDSemCmdi},
		CategoryRCE:   {score: sc.rce, id: RuleIDSemRCE},
		CategoryXXE:   {score: sc.xxe, id: RuleIDSemXXE},
		CategorySSRF:  {score: sc.ssrf, id: RuleIDSemSSRF},
		CategorySSTI:  {score: sc.ssti, id: RuleIDSemSSTI},
		CategoryExpr:  {score: sc.expr, id: RuleIDSemExpr},
		CategoryDeser: {score: sc.deser, id: RuleIDSemDeser},
	} {
		cats[cat] = st
	}

	for _, s := range scans {
		for cat, rules := range semRulesByCat() {
			if cats[cat].seen {
				continue
			}
			for _, r := range rules {
				text := s.text
				if r.onRaw {
					text = strings.ToLower(s.raw)
				}
				if m := r.re.FindString(text); m != "" {
					markHit(cat, s.target, m)
					break
				}
			}
		}
	}
	// Header 只扫表达式注入（jndi/SpEL/OGNL）
	if headerNorm != "" && !cats[CategoryExpr].seen {
		for _, r := range semExprRules {
			if m := r.re.FindString(headerNorm); m != "" {
				markHit(CategoryExpr, "header", m)
				break
			}
		}
	}

	res.ElapsedUS = time.Since(start).Microseconds()
	return res
}
