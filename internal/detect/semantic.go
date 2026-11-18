package detect

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// SemanticEngine 是第四检测通道：语义/结构判定，覆盖 SQLi/XSS（libinjection）之外的
// 命令注入、代码执行、XXE、SSRF、SSTI、表达式注入、反序列化，以及路径穿越、文件包含、
// NoSQL/LDAP/XPath 注入、CRLF/头注入、开放重定向、原型链污染、HTTP 走私、Webshell、
// GraphQL 攻击与 LLM 提示注入等 19 类（加上 libi 的 SQLi/XSS 共 21 类）。
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

// semScores 各类别的命中加分快照（Reload 时整体替换）。key 为类别名，
// 取值来自 semCatTable 定义的默认分；快照整体原子替换，读路径无锁。
type semScores struct {
	byCat map[string]int
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
	// —— v1.6.3 扩类（第一批：结构特征清晰）——
	DefaultSemTraverScore       = 35 // 路径穿越：../ 变体与敏感系统路径探测
	DefaultSemLFIScore          = 35 // 文件包含：非 PHP 伪协议 wrapper（php:// 在 rce 判）
	DefaultSemNoSQLiScore       = 40 // NoSQL 注入：Mongo 操作符 + db.xxx.find 结构
	DefaultSemLDAPiScore        = 40 // LDAP 注入：)( 闭合与 filter 结构
	DefaultSemXPathScore        = 40 // XPath 注入：'] or ' 闭合结构与节点计数
	DefaultSemCRLFScore         = 35 // CRLF/头注入：换行 + 头名结构（只判原文）
	DefaultSemOpenRedirectScore = 30 // 开放重定向：redirect 类参数跨域跳转（含宿主比对）
	DefaultSemPrototypeScore    = 40 // 原型链污染：__proto__/constructor.prototype 链
	DefaultSemSmuggScore        = 45 // HTTP 走私：TE/CL 冲突头组合（只判头）
	// —— 第二批：对抗性强，压低分值靠挑战而非拦截 ——
	DefaultSemWebshellScore  = 40 // Webshell：错误抑制 eval、/e 修饰符、变量函数调用
	DefaultSemGraphQLiScore  = 35 // GraphQL 攻击：内省查询、别名轰炸、批量查询数组
	DefaultSemPromptScore    = 30 // LLM 提示注入：指令覆盖/系统提示泄露结构
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
	RuleIDSemTraver       = "sem-traver"
	RuleIDSemLFI          = "sem-lfi"
	RuleIDSemNoSQLi       = "sem-nosqli"
	RuleIDSemLDAPi        = "sem-ldapi"
	RuleIDSemXPath        = "sem-xpati"
	RuleIDSemCRLF         = "sem-crlf"
	RuleIDSemOpenRedirect = "sem-openredirect"
	RuleIDSemPrototype    = "sem-prototype"
	RuleIDSemSmugg        = "sem-smugg"
	RuleIDSemWebshell     = "sem-webshell"
	RuleIDSemGraphQLi     = "sem-graphqli"
	RuleIDSemPrompt       = "sem-prompt"

	CategoryCmdi  = "cmdi"
	CategoryRCE   = "rce"
	CategorySSRF  = "ssrf"
	CategoryXXE   = "xxe"
	CategorySSTI  = "ssti"
	CategoryExpr  = "expr"
	CategoryDeser = "deser"
	CategoryTraver       = "traver"
	CategoryLFI          = "lfi"
	CategoryNoSQLi       = "nosqli"
	CategoryLDAPi        = "ldapi"
	CategoryXPath        = "xpati"
	CategoryCRLF         = "crlf"
	CategoryOpenRedirect = "openredirect"
	CategoryPrototype    = "prototype"
	CategorySmugg        = "smugg"
	CategoryWebshell     = "webshell"
	CategoryGraphQLi     = "graphqli"
	CategoryPrompt       = "prompt"
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
		// 高置信系统词：; | && ` $( 紧邻即判（不含 base64——data URI 的
		// ";base64," 是海量良性形态，实测 0.1% 误报预算的第一杀手）
		{re: mustRe(`[;&|]\s*(whoami|uname|ncat?\b|nc -|telnet|wget|curl|chmod|chown|xxd|strings|nslookup|dig |ifconfig|ipconfig|netstat|apt(?:-get)? |yum |pip[23]? |npm )`)},
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

	// semTraverRules 路径穿越：../ 重复变体与敏感系统路径探测。
	// 单个 ../ 在合法相对路径里会出现（redirect=../home），故要么重复两次以上，
	// 要么 ../ 或绝对路径指向已知敏感文件——两个条件都不成立的裸 ../ 不判。
	semTraverRules = []semRule{
		// 连续两次及以上 ../ 或 ..\（穿越目录树的典型结构）
		{re: mustRe(`(\.\.[/\\]){2,}`)},
		// ../ 后接敏感系统路径（一次穿越直接摸到系统文件）
		{re: mustRe(`\.\.[/\\][^=\s&]{0,40}(etc[/\\]|windows[/\\]|win\.ini|boot\.ini|root[/\\])`)},
		// 绝对路径探测经典目标（/etc/passwd、/etc/shadow、SAM 等强攻击指示物；
		// hosts/group/issue 在运维文档里太常见，不收）
		{re: mustRe(`(etc[/\\](passwd|shadow)|c:\\windows|windows[/\\]system32[/\\]config[/\\]sam|boot\.ini|win\.ini)`)},
		// 编码变体的连续穿越（%2e%2e 形态；双重编码 %252e 时 Normalize 还原不到，在原文判）
		{re: mustRe(`(%2e){2}(%2f|%5c|%2e)(%2e){2}`), onRaw: true},
	}

	// semLFIRules 文件包含：动态语言资源 wrapper 结构。
	// php://input/filter/expect/fd 已在 rce 判（直接指向代码执行语义），此处只收
	// 剩余 wrapper 与文件包含函数注入，避免同一 payload 双类计分。
	semLFIRules = []semRule{
		// PHP 剩余伪协议（zip/rar/phar 压缩流、data 流包装）
		{re: mustRe(`(zip|rar|phar|data|glob|rar)://[a-z0-9]`)},
		// php://filter 读源码的完整形态（base64 编码参数是其标志）
		{re: mustRe(`php://filter[^=\s]{0,80}(resource|read|convert)`)},
		// Java/ASP 文件流协议（netdoc、jar:、compress.zlib）
		{re: mustRe(`jar:(file|http)://|compress\.(zlib|bzip2)://|netdoc://`)},
	}

	// semNoSQLiRules NoSQL 注入：MongoDB 操作符与查询结构。
	// 关键字必须带 Mongo 语法形态（$ 前缀 + 结构分隔符 / db.xxx.find），裸词
	// "ne"/"in"/"or" 在普通 JSON/SQL 里太常见，一律不判。
	semNoSQLiRules = []semRule{
		// $where 带赋值/冒号（JS 注入入口，$where 结构最不易误报）
		{re: mustRe(`\$where\s*[:=\]]`)},
		// db 集合方法调用（Mongo shell / 注入点直调）
		{re: mustRe(`db\.[a-z_][a-z0-9_]*\.(find|aggregate|count|update|remove|drop|eval|mapreduce)\s*\(`)},
		// 强特征 Mongo 操作符 + 结构分隔符（JSON {"$ne":1}、URL user[$ne]=admin）；
		// 不收裸 $in（shell 变量 $in 误报面大，走下方 mangle 形态兜底）
		{re: mustRe(`\$(ne|gt|lt|gte|lte|nin|regex|where|elemmatch|comment|type)\s*[\]'{:,"]`)},
		// 弱操作符（or/and/not/exists）：只认 JSON 结构（冒号/取下标），不认空白
		{re: mustRe(`\$(or|and|not|exists)\s*[\]{:'"]`)},
		// PHP 风格参数 mangle：xxx[$operator]=value
		{re: mustRe(`\[\s*\$(ne|gt|lt|gte|lte|nin|in|regex|or|and|where)\s*\]`)},
		// 睡眠/布尔盲注的 Mongo 版（$where + sleep/Date 链）
		{re: mustRe(`\$where[^}]{0,40}(sleep|date\.now|new date|while\s*\(true)`)},
	}

	// semLDAPiRules LDAP 注入：filter 闭合结构。
	// 判定单元是 ")( 的闭合重组"——攻击者用 ) 关闭原 filter 再注入新分支，
	// 正常查询参数里 )( 重组几乎不存在（转义/括号成对出现的除外）。
	semLDAPiRules = []semRule{
		// *)(& 与 *)(| ：闭合后接新分支的经典形态
		{re: mustRe(`\*\)\s*[\(&|]`)},
		// )(uid=/)(cn= 等：闭合后注入新属性比较
		{re: mustRe(`\)\s*\(\s*(uid|cn|sn|mail|objectclass|dc|ou|memberof|samaccountname)\s*=`)},
		// 闭合后直接再闭合：*)(uid=*)) 的尾部
		{re: mustRe(`\)\s*\(\s*\)\s*[\)&|]`)},
		// DN 注入：,dc= 重组（cn=x,dc=y 的拼接攻击）
		{re: mustRe(`[\)\*&|][^()]{0,20},\s*(dc|ou|cn)\s*=`)},
	}

	// semXPathRules XPath 注入：节点闭合结构与通用节点函数。
	// count/string-length 只判原文（onRaw）：Normalize 会剥 SQL 注释形态的
	// /*，而 count(/*) 是 XPath 盲注的核心探测式，走原文不受影响。
	semXPathRules = []semRule{
		// ']' or ' 闭合引号结构（经典 ' or 'a'='a 的 XPath 版本）
		{re: mustRe(`['"]\]\s*(or|and)\s*['"]`)},
		// 闭合后接谓词/节点比较：')] 或 ](...)
		{re: mustRe(`\]\s*\[\s*(text\(\)|position\(\)|last\(\)|contains\(|not\()`)},
		// 节点计数/长度函数（XPath 盲注的探测原语）
		{re: mustRe(`count\s*\(\s*/|string-length\s*\(`), onRaw: true},
		// //node[not( / //node[contains( 等谓词直查（无引号闭合的形态）
		{re: mustRe(`//\*?\[\s*(not|text|position|last|contains)\s*\(`)},
		// //node[@attr=' 或 /*[contains( 结构
		{re: mustRe(`//\*?\[[^\]]{0,30}=\s*['"]|/\*\s*\[\s*contains\s*\(`)},
	}

	// semCRLFRules CRLF/头注入：换行 + 头名 的结构。
	// Normalize 会把换行压成空格，所以全部只判原文（onRaw）；且必须
	// "换行紧邻头名+冒号"——裸 %0d%0a 在编码过的正常文本里会出现。
	semCRLFRules = []semRule{
		// 已编码 CRLF 紧邻头名（URL 编码态在原文里就是 %0d%0a）
		{re: mustRe(`%0d%0a\s*[a-z][a-z0-9-]{2,20}:|%0a%0d\s*[a-z][a-z0-9-]{2,20}:`), onRaw: true},
		// 真实 CRLF 紧邻响应头名（头拆分的最终形态）
		{re: mustRe(`[\r\n]\s*(location|set-cookie|content-length|content-type|content-disposition|x-forwarded|x-injected|host)\s*:`), onRaw: true},
		// CRLF + 注入标记头（X-Forwarded-For 伪造与自造头的组合）
		{re: mustRe(`[\r\n]%0d?%0a?\s*(x-[a-z-]{2,20})\s*:`), onRaw: true},
	}

	// semPrototypeRules 原型链污染：__proto__ / constructor.prototype 链。
	semPrototypeRules = []semRule{
		// __proto__ 键 + 赋值/取值结构（JSON key 或 JS 表达式）
		{re: mustRe(`__proto__\s*[\[:."']=]|["']__proto__["']\s*:`)},
		// constructor.prototype 链式赋值（无 __proto__ 时的等价路径）
		{re: mustRe(`constructor\s*(\.\s*prototype|\[\s*['"]prototype['"]\s*\])\s*[\[.\["'=]`)},
		// 递归合并入口：merge/deepmerge/extend + __proto__（漏洞模式组合）
		{re: mustRe(`(merge|deepmerge|deepmergeall|extend|defaultsdeep)\s*\([^)]{0,60}__proto__`)},
		// prototype[xxx] 动态属性链
		{re: mustRe(`prototype\s*\[\s*['"][a-z_]+['"]\s*\]\s*=`)},
	}

	// semWebshellRules Webshell 特征：错误抑制执行、/e 修饰符、变量函数调用。
	// 与 rce 的分工：rce 收"系统函数调用"，webshell 收"混淆/回传执行"的
	// 一句话木马形态——@ 前缀、动态调用、请求超参直接当函数跑。
	semWebshellRules = []semRule{
		// 错误抑制符 + 动态执行（@eval/@assert 是一句话木马的标志形态）
		{re: mustRe(`@\s*(eval|assert|preg_replace|create_function|system|passthru|shell_exec)\s*\(`)},
		// preg_replace /e 修饰符（PHP <7 的隐藏代码执行）
		{re: mustRe(`preg_replace\s*\(\s*['"][^'"]{0,60}[\da-z]{0,6}/e['"]`)},
		// 请求超参当函数调（$_POST['cmd']() / $_GET['f']($_GET['x'])）
		{re: mustRe(`\$_(get|post|request|cookie)\s*\[[^\]]{1,30}\]\s*\(`)},
		// create_function / call_user_func 执行不可信串（create_function 本身
		// 就是 PHP 7.2 前的动态代码执行入口，单出现即判）
		{re: mustRe(`create_function\s*\(`)},
		{re: mustRe(`call_user_func(_array)?\s*\(\s*['"]?(assert|eval|system|passthru|shell_exec|base64_decode|gzinflate)`)},
		// 变量函数双跳板（$f=$_GET[...]; $f(...) 的常见中转写法；
		// RE2 无反向引用，"同变量"约束用赋值-调用相邻的形态近似）
		{re: mustRe(`\$\w+\s*=\s*\$_(get|post|request)[^;]{0,40};[\s\S]{0,40}\$\w+\s*\(`)},
	}

	// semGraphQLiRules GraphQL 攻击：内省、别名轰炸、批量查询数组。
	semGraphQLiRules = []semRule{
		// 内省查询（__schema/__type 是攻击枚举的入口）
		{re: mustRe(`__schema\s*\{|__type\s*\(|__introspection\b`)},
		// 查询体内引用 __schema（完整内省文档）
		{re: mustRe(`(query|mutation|subscription)\s*\{[^{}]{0,240}__schema`)},
		// 别名轰炸：连续 ≥10 个 "alias: field {"（正常查询不会连续堆叠）
		{re: mustRe(`(\w+\s*:\s*\w+\s*(\([^)]{0,40}\))?\s*\{){10,}`)},
		// 批量查询数组：JSON 数组里连续多个 "query" 键
		{re: mustRe(`\[\s*\{\s*"query"\s*:|(\{\s*"query"\s*:[,}\s]{1,4}\s*){3,}`)},
		// 字段 suggestion 泄露探测与 arg 注入结构
		{re: mustRe(`\(\s*(where|and|or|nor|options|filters?)\s*:\s*["'\{]`)},
	}

	// semPromptRules LLM 提示注入：指令覆盖与系统提示泄露的结构短语。
	// 全部要求"动词 + 约束对象"完整短语，避免新闻/博客讨论里半截语句误报。
	semPromptRules = []semRule{
		// 指令覆盖：ignore/disregard/forget + previous/prior/above + instructions
		{re: mustRe(`(ignore|disregard|forget|bypass|override)\s+(all\s+|any\s+)?(previous|prior|above|earlier|preceding)\s+(instructions|prompts?|rules?|directions?|messages?|constraints?)`)},
		// 系统提示泄露：动词 + system prompt 完整短语
		{re: mustRe(`(reveal|show|print|display|output|repeat|leak|expose|disclose)\s+(your\s+|the\s+|its\s+)?(initial\s+|hidden\s+|full\s+|original\s+|above\s+)?(system\s+prompt|developer\s+message|initial\s+instructions)`)},
		// 角色重写：从现在起你是 / pretend you are + 可执行动作链
		{re: mustRe(`(pretend\s+(that\s+)?you\s+are|you\s+are\s+now\s+(a|an|the)|act\s+as\s+(if|a|an))[^.;]{0,60}(ignore|forget|no\s+rules|without\s+restrictions|jailbreak)`)},
		// 越狱模板标志词（DAN/do-anything-now + 启用短语；"developer mode"
		// 在 Chrome/Discord 语境太常见，不收）
		{re: mustRe(`(do\s+anything\s+now|(dan|jailbreak)\s+mode)[^.;]{0,40}(enabled|activated|on\b)`)},
	}
)

// semCatDef 类别定义：单一事实来源——规则 ID、中文名、默认分都从这里出。
type semCatDef struct {
	cat     string // 类别名（Hit.Category）
	ruleID  string // 规则 ID（例外规则按此精确豁免）
	name    string // 中文名（事件展示用）
	defaultScore int // 默认命中加分（engine.sem.<cat>_score 可覆盖）
}

// semCatTable 全部语义类别，按"高危在前"排序；Inspect 按此顺序扫描与记录，
// 保证事件里命中列表的确定性。
func semCatTable() []semCatDef {
	return []semCatDef{
		{CategoryRCE, RuleIDSemRCE, "远程代码执行", DefaultSemRCEScore},
		{CategoryCmdi, RuleIDSemCmdi, "命令注入", DefaultSemCmdiScore},
		{CategoryExpr, RuleIDSemExpr, "表达式/语言注入", DefaultSemExprScore},
		{CategoryDeser, RuleIDSemDeser, "反序列化载荷", DefaultSemDeserScore},
		{CategoryXXE, RuleIDSemXXE, "XXE 外部实体注入", DefaultSemXXEScore},
		{CategorySSTI, RuleIDSemSSTI, "SSTI 模板注入", DefaultSemSSTIScore},
		{CategorySSRF, RuleIDSemSSRF, "SSRF 服务端请求伪造", DefaultSemSSRFScore},
		{CategorySmugg, RuleIDSemSmugg, "HTTP 走私", DefaultSemSmuggScore},
		{CategoryWebshell, RuleIDSemWebshell, "Webshell 一句话木马", DefaultSemWebshellScore},
		{CategoryNoSQLi, RuleIDSemNoSQLi, "NoSQL 注入", DefaultSemNoSQLiScore},
		{CategoryLDAPi, RuleIDSemLDAPi, "LDAP 注入", DefaultSemLDAPiScore},
		{CategoryXPath, RuleIDSemXPath, "XPath 注入", DefaultSemXPathScore},
		{CategoryPrototype, RuleIDSemPrototype, "原型链污染", DefaultSemPrototypeScore},
		{CategoryLFI, RuleIDSemLFI, "文件包含", DefaultSemLFIScore},
		{CategoryTraver, RuleIDSemTraver, "路径穿越", DefaultSemTraverScore},
		{CategoryCRLF, RuleIDSemCRLF, "CRLF/头注入", DefaultSemCRLFScore},
		{CategoryGraphQLi, RuleIDSemGraphQLi, "GraphQL 攻击", DefaultSemGraphQLiScore},
		{CategoryOpenRedirect, RuleIDSemOpenRedirect, "开放重定向", DefaultSemOpenRedirectScore},
		{CategoryPrompt, RuleIDSemPrompt, "LLM 提示注入", DefaultSemPromptScore},
	}
}

// semRulesByCat 各类别的结构判定式（key 必须与 semCatTable 的 cat 一致）。
func semRulesByCat() map[string][]semRule {
	return map[string][]semRule{
		CategoryCmdi:  semCmdiRules,
		CategoryRCE:   semRCERules,
		CategoryXXE:   semXXERules,
		CategorySSRF:  semSSRFRules,
		CategorySSTI:  semSSTIRules,
		CategoryExpr:  semExprRules,
		CategoryDeser: semDeserRules,
		CategoryTraver:       semTraverRules,
		CategoryLFI:          semLFIRules,
		CategoryNoSQLi:       semNoSQLiRules,
		CategoryLDAPi:        semLDAPiRules,
		CategoryXPath:        semXPathRules,
		CategoryCRLF:         semCRLFRules,
		CategoryPrototype:    semPrototypeRules,
		CategoryWebshell:     semWebshellRules,
		CategoryGraphQLi:     semGraphQLiRules,
		CategoryPrompt:       semPromptRules,
		// openredirect / smugg 走 semCheckOpenRedirect / semCheckSmugg 结构化检查，
		// 不是纯文本正则（分别需要宿主比对与头组合分析），不进本表。
	}
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

// setScores 装配分值快照（内部方法）。默认分取自 semCatTable，
// scores 里显式给出且 >0 的条目覆盖默认值；未知类别忽略。
func (e *SemanticEngine) setScores(scores map[string]int) {
	byCat := make(map[string]int, len(semCatTable()))
	for _, def := range semCatTable() {
		byCat[def.cat] = def.defaultScore
	}
	for cat, v := range scores {
		if _, known := byCat[cat]; known && v > 0 {
			byCat[cat] = v
		}
	}
	e.scores.Store(&semScores{byCat: byCat})
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
		name  string
		seen  bool
	}
	cats := map[string]*catState{}
	table := semCatTable()
	for _, def := range table {
		cats[def.cat] = &catState{score: sc.byCat[def.cat], id: def.ruleID, name: def.name}
	}
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
			Name:     "语义识别" + st.name,
			Category: cat,
			Score:    st.score,
			Target:   target,
			Evidence: Truncate(evidence, 120),
		})
		res.RuleIDs = append(res.RuleIDs, st.id)
		res.Categories = appendUnique(res.Categories, cat)
	}

	rulesByCat := semRulesByCat()
	for _, s := range scans {
		for _, def := range table {
			if cats[def.cat].seen {
				continue
			}
			for _, r := range rulesByCat[def.cat] {
				text := s.text
				if r.onRaw {
					text = strings.ToLower(s.raw)
				}
				if m := r.re.FindString(text); m != "" {
					markHit(def.cat, s.target, m)
					break
				}
			}
		}
	}
	// 结构化检查：HTTP 走私（头组合分析）与开放重定向（跨域宿主比对）——
	// 两类特征不是单条文本正则能表达的，走专用函数，同样只记一次命中。
	if ev, ok := semCheckSmugg(in); ok {
		markHit(CategorySmugg, "header", ev)
	}
	if ev, ok := semCheckOpenRedirect(in); ok {
		markHit(CategoryOpenRedirect, "query", ev)
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

// semRedirectParamRe 开放重定向的参数形态：redirect 类参数名 + 绝对 URL。
// 宿主捕获停在端口/路径前（[a-z0-9._-]+ 不含冒号），(?i) 由 RE2 原生支持；
// = 后允许引号（quote-pad 变体是真实攻击形态）。
var semRedirectParamRe = mustRe(`(?i)(redirect|redirect_uri|redirecturl|returnurl|return_to|return|rurl|url|next|goto|target|dest|destination|continue|callback|jump|link|forward|out)=['"]?https?://([a-z0-9._-]+)`)

// semCheckOpenRedirect 开放重定向的跨域判定：redirect 类参数指向与站点
// 自身不同的主机才算命中。同域互跳（含 www. 前缀与子域，按点边界判定）
// 是合法业务行为；本站域名未知（Host 为空）时放弃判定，宁漏勿误。
// query 与 body（form 表单 POST 重定向）都查，原文与归一化各一遍。
func semCheckOpenRedirect(in *Input) (string, bool) {
	if in.Host == "" {
		return "", false
	}
	own := strings.ToLower(in.Host)
	if i := strings.IndexByte(own, ':'); i >= 0 {
		own = own[:i]
	}
	if own == "" {
		return "", false
	}
	// 原文、归一化与解码变体各查一遍：URL 编码（%3A%2F%）、unicode 转义
	// 与 base64 整包形态只有解码后才可读，重复检查无副作用（同类只记一次）。
	var texts []string
	appendTexts := func(raw string, variants []string) {
		if raw != "" {
			texts = append(texts, raw, Normalize(raw))
		}
		for _, v := range variants {
			if v != "" {
				texts = append(texts, v, Normalize(v))
			}
		}
	}
	appendTexts(in.Query, in.DecodedQuery)
	appendTexts(in.Body, in.DecodedBody)
	for _, text := range texts {
		m := semRedirectParamRe.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		target := strings.ToLower(m[2])
		target = strings.TrimSuffix(target, ".")
		if target == own || strings.HasSuffix(target, "."+own) || strings.HasSuffix(own, "."+target) {
			continue
		}
		return m[0], true
	}
	return "", false
}

// semCheckSmugg HTTP 走私的头组合判定：CL 头重复出现，或 TE 与 CL 并存。
// 两类组合都是经典走私前奏（前端服务器与后端对请求边界的理解不一致），
// 正常客户端请求不会出现，误报面极小。
func semCheckSmugg(in *Input) (string, bool) {
	h := strings.ToLower(in.Headers)
	if h == "" {
		return "", false
	}
	clCount := strings.Count(h, "content-length:")
	teCount := strings.Count(h, "transfer-encoding:")
	switch {
	case clCount >= 2:
		return fmt.Sprintf("content-length 头出现 %d 次（请求边界歧义）", clCount), true
	case teCount > 0 && clCount > 0:
		return "transfer-encoding 与 content-length 同时出现（TE/CL 冲突）", true
	}
	return "", false
}
