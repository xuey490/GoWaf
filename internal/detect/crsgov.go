package detect

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"sync"

	crsv4 "github.com/corazawaf/coraza-coreruleset/crs/v4"
)

// crsv4FS 返回内嵌 CRS 资源根 FS（供字面量提取与规则 ID 枚举共用）。
func crsv4FS() fs.FS { return crsv4.FS }

// CRSGov 是 CRS 第二引擎的「调度闸门」：决定一次请求要不要完整跑 Coraza。
//
// 实测背景（docs/v1.5.0-与雷池社区版对比.md）：全引擎吞吐 4.3k RPS，关掉 CRS
// 直接涨到 17.1k——CRS 事务（数百条规则 × 每请求一次）是吞吐天花板的唯一主因。
// 但 CRS 不能整体关，闸门提供三个有界、可解释的旁路：
//
//  1. exclude_paths：管理员显式豁免的路径前缀（如 /health、内网回调）；
//  2. skip_static：GET/HEAD、无 query、无 body、扩展名在静态清单内的请求跳过
//     ——静态资源请求不含用户输入参数，CRS 的 ARGS 检测天然无事可做；
//  3. prescreen：可疑度预筛。用 AC 自动机（关键词取自 CRS 规则文件的字面量
//     + 扫描器 UA 词表）扫 path/query/body/headers，一个都不命中才跳过。
//     这是「加速模式」：CRS 里少量纯结构性规则（如 @detectXSS）没有可提取的
//     字面量，开启预筛意味着接受这批规则被旁路的代价，因此默认关闭。
//
// 闸门本身无状态、并发安全；ShouldInspect 是纯函数。
type CRSGov struct {
	prescreen    bool                // 可疑度预筛开关（默认关）
	skipStatic   bool                // 静态资源旁路（默认开）
	bodyTextOnly bool                // 非 text 请求体跳过 CRS body 解析（默认开）
	excludePaths []string            // 路径前缀豁免清单
	ac           *AC                 // 预筛关键词自动机（nil 表示预筛不可用，按开启处理）
	staticExts   map[string]struct{} // 静态扩展名集合（小写，含点前缀去除）
}

// CRSGovOptions 构造参数。
type CRSGovOptions struct {
	Prescreen    bool
	SkipStatic   bool
	BodyTextOnly bool
	ExcludePaths []string
}

// defaultStaticExts 默认静态扩展名清单：纯资源类，几乎不可能携带业务参数。
// 不含 pdf/docx 等下载型文件——它们常由脚本生成，误旁路的代价高。
var defaultStaticExts = []string{
	".css", ".js", ".mjs", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp",
	".svg", ".ico", ".avif", ".bmp", ".woff", ".woff2", ".ttf", ".eot", ".otf",
	".mp4", ".mp3", ".webm", ".ogg", ".wav", ".m3u8", ".ts",
}

// curatePrescreenKeywords 预筛补充词表：CRS 文件里提取不到或提取不全的
// 高价值特征（扫描器/爆破工具 UA、协议攻击关键字）。小写。
var curatePrescreenKeywords = []string{
	"sqlmap", "nikto", "nessus", "netsparker", "acunetix", "masscan", "nmap",
	"zgrab", "zmap", "dirbuster", "dirb", "gobuster", "wfuzz", "ffuf", "feroxbuster",
	"burpsuite", "burp collaborator", "wpscan", "havij", "hydra", "medusa",
	"metasploit", "msf", "w3af", "arachni", "openvas", "qualys", "whatweb",
	"sqlninja", "bsqlbf", "pornspider", "ads_click", "w8scan",
	// 协议/走私类关键字（921xxx 的部分规则是纯结构，补字面量）
	"chunked;e", ";chunked", "transfer-foo", "content-length:0", "content-length: 0",
	// webshell 常见落点（930/933 提取不到的部分）
	"webconsole", "c99shell", "r57shell", "wso.php", "b374k", "alfa-xshell",
	// 经典注入结构：@detectSQLi/@detectXSS 类规则无字面量可提取，
	// 用高频 payload 结构补齐（这正是预筛最大的已知盲区，见 ShouldInspect 注释）
	"1'='1", "1=1", "' or '", "\" or \"", "or 1=1", "and 1=1", "union all select",
	"sleep(", "benchmark(", "waitfor delay", "information_schema", "load_file(",
	"outfile", "dumpfile", "<script", "onerror=", "onload=", "javascript:",
	"alert(", "prompt(", "confirm(", "document.cookie", "expression(",
}

// crsLiteralOnce 保证 CRS 字面量只提取一次（进程级缓存，构建 ~10ms 级）。
var (
	crsLiteralOnce sync.Once
	crsLiterals    []string
)

// noiseWords 预筛噪声词：HTTP/规则引擎的通用词汇与正常浏览器特征，
// 几乎所有请求都会出现，提取出来只会让预筛永远命中、失去旁路意义。
var noiseList = []string{
	// HTTP 与规则引擎词汇
	"http", "https", "html", "host", "user", "agent", "referer", "content",
	"length", "type", "accept", "charset", "encoding", "connection", "cookie",
	"header", "headers", "request", "response", "server", "method", "query",
	"string", "match", "regex", "variable", "value", "false", "true", "null",
	"none", "error", "timeout", "status", "range", "pragma", "cache", "control",
	"origin", "token", "scheme", "form", "data", "multipart", "boundary",
	"args", "post", "get", "self", "this", "file", "name", "path", "text",
	"line", "bytes", "case", "global", "common", "config", "default",
	"sec", "rule", "action", "phase", "audit", "engine", "libinjection",
	// 正常浏览器/系统特征（CRS 扫描器规则的排除项里大量出现）
	"mozilla", "applewebkit", "webkit", "safari", "chrome", "gecko",
	"firefox", "windows", "macintosh", "android", "linux", "iphone",
	"example", "sample", "test",
	// 常见请求词汇：每个正常请求都会出现（localhost、Accept 的
	// application/*、UA 的 like Gecko、域名 www 等）
	"local", "application", "like", "www", "xhtml", "json", "image",
	"fonts", "static", "assets", "index", "main", "style", "script",
}

// isNoisy 判断提取段是否为噪声：等于噪声词、是噪声词的子串、或包含噪声词
// 都算（"accep"⊂accept、"applewebkit"⊃webkit、"windowsystem32"⊃windows）。
// 包含方向的误伤面很小：含这些词的攻击特征段极少，且预筛只是加速模式。
func isNoisy(seg string) bool {
	for _, n := range noiseList {
		if seg == n || strings.Contains(seg, n) || strings.Contains(n, seg) {
			return true
		}
	}
	return false
}

// extractCRSLiterals 从内嵌 CRS 规则文件提取预筛关键词。
// 思路：逐条 SecRule 的 @rx 模式取「长度 ≥4 的连续字母数字下划线段」——
// 模式里写死的字面量（union、select、base64_decode、/etc/passwd 的一段…）
// 都会被提出来；纯结构模式（\d{4} 之类）提不出段，属于预筛的固有盲区，
// 由开关默认关闭来兜底。返回小写去重列表。
func extractCRSLiterals() []string {
	crsLiteralOnce.Do(func() {
		segRe := regexp.MustCompile(`[a-z0-9_]{4,}`)
		set := map[string]struct{}{}
		files, err := fs.ReadDir(crsv4FS(), "@owasp_crs")
		if err != nil {
			crsLiterals = nil
			return
		}
		names := make([]string, 0, len(files))
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".conf") {
				names = append(names, f.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			raw, err := fs.ReadFile(crsv4FS(), "@owasp_crs/"+name)
			if err != nil {
				continue
			}
			for _, m := range crsOpRe.FindAllSubmatch(raw, -1) {
				if len(m[2]) == 0 {
					continue
				}
				op := strings.ToLower(string(m[2]))
				switch string(m[1]) {
				case "@contains":
					// @contains 的参数整体就是字面量
					if len(op) >= 4 {
						set[op] = struct{}{}
					}
				default: // @rx 及省略 @rx 的裸模式
					for _, seg := range segRe.FindAllString(op, -1) {
						if len(seg) >= 4 && !isNoisy(seg) {
							set[seg] = struct{}{}
						}
					}
				}
			}
		}
		out := make([]string, 0, len(set)+len(curatePrescreenKeywords))
		for k := range set {
			out = append(out, k)
		}
		out = append(out, curatePrescreenKeywords...)
		crsLiterals = out
	})
	return crsLiterals
}

// crsOpRe 匹配 SecRule 里的操作符段：@rx 模式、@contains 参数或裸正则。
// 只取第二捕获组（模式/参数本体）；限定在引号内的操作符位置，避免把
// 变量名/动作名误当模式。
var crsOpRe = regexp.MustCompile(`(?i)@(rx|contains|detectsqli|detectxss|pm|pmf|streq|rsub|within)\s+((?:[^"\\]|\\.)*)`)

// NewCRSGov 构建闸门快照。
// 参数 opt：三个旁路开关与豁免清单。
// 返回 *CRSGov：预筛关键词构建失败时 ac 为 nil（预筛自动按「不可用→不旁路」处理）。
func NewCRSGov(opt CRSGovOptions) *CRSGov {
	g := &CRSGov{
		prescreen:    opt.Prescreen,
		skipStatic:   opt.SkipStatic,
		bodyTextOnly: opt.BodyTextOnly,
		excludePaths: append([]string(nil), opt.ExcludePaths...),
		staticExts:   map[string]struct{}{},
	}
	for _, e := range defaultStaticExts {
		g.staticExts[e] = struct{}{}
	}
	for i, p := range g.excludePaths {
		g.excludePaths[i] = strings.TrimSpace(p)
	}
	// 清掉空项与结尾斜杠，统一按前缀匹配
	filtered := g.excludePaths[:0]
	for _, p := range g.excludePaths {
		if p == "" {
			continue
		}
		filtered = append(filtered, strings.TrimSuffix(p, "/"))
	}
	g.excludePaths = filtered
	if g.prescreen {
		lits := extractCRSLiterals()
		if len(lits) > 0 {
			g.ac = NewAC(lits)
		}
	}
	return g
}

// BodyTextOnly 报告是否只对文本类请求体跑 CRS body 解析。
func (g *CRSGov) BodyTextOnly() bool { return g != nil && g.bodyTextOnly }

// isStaticPath 判断路径是否命中静态扩展名清单。
func (g *CRSGov) isStaticPath(path string) bool {
	dot := strings.LastIndexByte(path, '.')
	if dot < 0 || dot == len(path)-1 {
		return false
	}
	_, ok := g.staticExts[strings.ToLower(path[dot:])]
	return ok
}

// isExcluded 判断路径是否命中豁免前缀。
func (g *CRSGov) isExcluded(path string) bool {
	for _, p := range g.excludePaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// ShouldInspect 判断本次请求是否需要跑完整 CRS。
// 参数 site：站点名（保留参数，供未来按站点细分）；method：HTTP 方法；
// path/query/body：请求路径、查询串与请求体；headersText：已拼好的
// "K: v" 头串（与 detect.Input.Headers 同格式，含 UA）。
// 返回 (bool, string)：是否需要检查；跳过时返回跳过原因（供指标归因），
// 需要检查时原因为空串。
func (g *CRSGov) ShouldInspect(site, method, path, query, body, headersText string) (bool, string) {
	if g == nil {
		return true, ""
	}
	if g.isExcluded(path) {
		return false, "excluded_path"
	}
	// 静态资源旁路：无参数的纯资源 GET/HEAD，CRS 无 ARGS 可检
	if g.skipStatic && len(body) == 0 && query == "" &&
		(method == "GET" || method == "HEAD" || method == "OPTIONS") &&
		g.isStaticPath(path) {
		return false, "static_asset"
	}
	// 可疑度预筛：AC 一遍扫过 path/query/body/headers，零命中才旁路
	if g.prescreen && g.ac != nil {
		text := Normalize(path + " " + query + " " + body + " " + headersText)
		if len(g.ac.Match(text)) == 0 {
			return false, "prescreen"
		}
	}
	return true, ""
}

// PrescreenKeywords 返回预筛关键词数量（控制台展示用）。
func (g *CRSGov) PrescreenKeywords() int {
	if g == nil || g.ac == nil {
		return 0
	}
	return g.ac.Size()
}

// PrescreenHits 返回文本命中的预筛关键词（诊断用）。
// 参数 text：待检查文本。返回 []string：命中的关键词列表。
func (g *CRSGov) PrescreenHits(text string) []string {
	if g == nil || g.ac == nil {
		return nil
	}
	kws := g.ac.Keywords()
	var out []string
	for _, idx := range g.ac.Match(Normalize(text)) {
		if idx >= 0 && idx < len(kws) {
			out = append(out, kws[idx])
		}
	}
	return out
}

// ValidateRemoveRanges 校验并清洗规则号段移除配置（导出供 runtime 转发调用）。
// 参数 v：逗号分隔的 3 位号段前缀（如 "920,921"）。
// 返回 []string：清洗后的号段列表；空输入返回 nil。
// 仅接受 910-949 号段（CRS 检测规则区间），其他输入丢弃。
func ValidateRemoveRanges(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if len(p) != 3 {
			continue
		}
		n := 0
		if _, err := fmt.Sscanf(p, "%3d", &n); err != nil || n < 910 || n > 949 {
			continue
		}
		dup := false
		for _, e := range out {
			if e == p {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

// crsIDRe 从 SecLang 文本提取 6 位规则 ID。
var crsIDRe = regexp.MustCompile(`\bid\s*[:=]\s*'?(\d{6})`)

// crsRuleIDs 读取内嵌 CRS 全部规则 ID（进程级缓存）。
// 返回 map[int]struct{}：规则 ID 集合；读取失败返回空集（等于不移除任何规则）。
func crsRuleIDs() map[int]struct{} {
	ids := map[int]struct{}{}
	files, err := fs.ReadDir(crsv4FS(), "@owasp_crs")
	if err != nil {
		return ids
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".conf") {
			continue
		}
		raw, err := fs.ReadFile(crsv4FS(), "@owasp_crs/"+f.Name())
		if err != nil {
			continue
		}
		for _, m := range crsIDRe.FindAllStringSubmatch(string(raw), -1) {
			var id int
			if _, err := fmt.Sscanf(m[1], "%d", &id); err == nil {
				ids[id] = struct{}{}
			}
		}
	}
	return ids
}

// removalDirectives 按「移除号段」生成 SecRuleRemoveById 指令行。
// 参数 ranges：3 位号段前缀列表（已校验）。
// 返回 string：形如 "SecRuleRemoveById 920100\n…" 的指令块；无号段返回空串。
func removalDirectives(ranges []string) string {
	if len(ranges) == 0 {
		return ""
	}
	ids := crsRuleIDs()
	if len(ids) == 0 {
		return ""
	}
	var matched []int
	for id := range ids {
		for _, r := range ranges {
			lo := (int(r[0]-'0')*100 + int(r[1]-'0')*10 + int(r[2]-'0')) * 1000
			if id >= lo && id < lo+1000 {
				matched = append(matched, id)
				break
			}
		}
	}
	if len(matched) == 0 {
		return ""
	}
	sort.Ints(matched)
	var sb strings.Builder
	for _, id := range matched {
		fmt.Fprintf(&sb, "SecRuleRemoveById %d\n", id)
	}
	return sb.String()
}
