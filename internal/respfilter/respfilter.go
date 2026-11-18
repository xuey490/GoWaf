// Package respfilter 实现响应体敏感信息过滤（雷池专业版同款能力的开源实现）。
//
// 请求侧检测再严密，也拦不住上游应用自己把敏感数据吐出去：调试接口打印手机号、
// 报错页带出数据库连接串、日志接口泄漏云密钥。本包在响应回传链路上对 2xx 的
// 文本类响应做一次扫描：
//
//	observe 模式（默认）：只记录命中、不改写内容，先跑一段看误报；
//	mask    模式：把命中的敏感片段就地打码后下发，同时记录事件。
//
// 设计约束：
//  1. 只处理有界的文本响应（≤ max_bytes），超大/流式/压缩响应直接透传，
//     绝不为了扫描把视频或大文件读进内存；
//  2. 正则全部是 ASCII 词边界锚定，直接在 UTF-8 字节上执行，不做整页解码；
//  3. 命中永远不参与评分与拦截（Score 恒为 0），本模块只"看"和"改"，不"判"。
package respfilter

import (
	"bytes"
	"regexp"
	"strings"

	"gowaf/internal/detect"
)

// Settings 响应敏感信息过滤配置，由 Reload 时从设置表编译，数据面只读。
type Settings struct {
	Enabled      bool     // 总开关，默认关闭（先观察再启用）
	Mode         string   // observe 只记录 / mask 脱敏
	MaxBytes     int      // 参与扫描的响应体上限（字节），超过直接透传
	ExcludePaths []string // 不扫描的路径前缀（如 /api/export）
	Categories   []string // 启用的敏感信息类别
}

// 支持的敏感信息类别。categories 设置键只能取这些值，未知值在写入侧被拒绝。
const (
	CatPhone       = "phone"       // 中国大陆手机号
	CatIDCard      = "idcard"      // 二代身份证号（18 位）
	CatBankCard    = "bankcard"    // 银联卡号（62 开头 16~19 位）
	CatCredentials = "credentials" // 云厂商/服务 API 密钥与私钥块
	CatJWT         = "jwt"         // JWT 令牌
	CatEmail       = "email"       // 邮箱地址（误报率高，默认关闭）
)

// KnownCategories 返回全部受支持的类别。
// 无参数；返回 []string：类别名列表。
func KnownCategories() []string {
	return []string{CatPhone, CatIDCard, CatBankCard, CatCredentials, CatJWT, CatEmail}
}

// DefaultCategories 默认启用的类别：邮箱误报率太高（页面里出现别人邮箱属正常业务），
// 不进默认集。
func DefaultCategories() []string {
	return []string{CatPhone, CatIDCard, CatBankCard, CatCredentials, CatJWT}
}

// DefaultMaxBytes 默认扫描上限：1MB。绝大多数 HTML/JSON 响应在 1MB 以内，
// 超过的基本是大导出/大页面，扫描收益低、内存代价高。
const DefaultMaxBytes = 1 << 20

// DefaultSettings 返回默认配置（关闭、观察模式）。
// 无参数；返回 Settings：首次启用前的安全默认值。
func DefaultSettings() Settings {
	return Settings{
		Enabled:    false,
		Mode:       "observe",
		MaxBytes:   DefaultMaxBytes,
		Categories: DefaultCategories(),
	}
}

// rule 一条敏感信息规则：正则 + 打码函数。
//
// 正则统一用 "边界组 + 内容组 + 边界组" 的三段式（RE2 不支持 lookbehind），
// 第一、三组用于断言前后不是字母数字，避免把 11234567890123 里的 11 位截出来；
// 打码函数只处理中间的内容组，边界组原样保留。
type rule struct {
	id       string                  // 事件里的规则标识，如 resp-phone
	category string                  // 所属类别
	re       *regexp.Regexp          // 已编译正则
	mask     func(m [][]byte) []byte // 打码函数：入参为 FindAllSubmatchIndex 的字节切片（含边界组）
}

// 边界组：前后不能是字母或数字（含下划线类似词字符，防 UUID/长串里误匹配）。
const (
	boundaryPre  = `(^|[^0-9A-Za-z])`
	boundaryPost = `($|[^0-9A-Za-z])`
)

// rules 内置规则表。首次使用时惰性编译（包级单例，编译一次全程复用）。
var rules = []rule{
	{
		id: "resp-phone", category: CatPhone,
		re: regexp.MustCompile(boundaryPre + `(1[3-9]\d{9})` + boundaryPost),
		mask: func(m [][]byte) []byte {
			p := m[2]
			return append(append(append([]byte{}, p[0:3]...), []byte("****")...), p[7:11]...)
		},
	},
	{
		id: "resp-idcard", category: CatIDCard,
		re: regexp.MustCompile(boundaryPre + `([1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx])` + boundaryPost),
		mask: func(m [][]byte) []byte {
			p := m[2]
			return append(append(append([]byte{}, p[0:4]...), []byte("**********")...), p[14:18]...)
		},
	},
	{
		id: "resp-bankcard", category: CatBankCard,
		re: regexp.MustCompile(boundaryPre + `(62\d{14,17})` + boundaryPost),
		mask: func(m [][]byte) []byte {
			p := m[2]
			return append(append(append([]byte{}, p[0:4]...), []byte("********")...), p[len(p)-4:]...)
		},
	},
	{
		id: "resp-credentials", category: CatCredentials,
		// 一条大正则覆盖常见的云密钥形态：AWS AK、OpenAI sk-、GitHub gh*_、
		// 阿里云 LTAI、Google AIza、PEM 私钥块。命中统一替换为占位符。
		re: regexp.MustCompile(`(?i)((?:AKIA[0-9A-Z]{16})|(?:sk-[A-Za-z0-9_-]{20,})|(?:gh[pousr]_[A-Za-z0-9]{30,})|(?:LTAI[A-Za-z0-9]{12,})|(?:AIza[0-9A-Za-z_\-]{35})|(?:-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY(?: BLOCK)?-----))`),
		mask: func(m [][]byte) []byte {
			return []byte("[REDACTED]")
		},
	},
	{
		id: "resp-jwt", category: CatJWT,
		re: regexp.MustCompile(boundaryPre + `(eyJ[A-Za-z0-9_\-]{8,}\.eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,})` + boundaryPost),
		mask: func(m [][]byte) []byte {
			return []byte("[REDACTED-JWT]")
		},
	},
	{
		id: "resp-email", category: CatEmail,
		re: regexp.MustCompile(boundaryPre + `([A-Za-z0-9._%+\-]{1,64}@[A-Za-z0-9.\-]{2,255}\.[A-Za-z]{2,})` + boundaryPost),
		mask: func(m [][]byte) []byte {
			p := m[2]
			at := bytes.IndexByte(p, '@')
			if at < 0 {
				return []byte("[REDACTED]")
			}
			head := p[0:1]
			if len(p) >= 2 {
				head = p[0:2]
			}
			out := append(append([]byte{}, head...), []byte("***")...)
			return append(out, p[at:]...)
		},
	},
}

// ruleByID 按 id 取规则，供按类别启用时查找。
var ruleByID = func() map[string]*rule {
	m := make(map[string]*rule, len(rules))
	for i := range rules {
		m[rules[i].id] = &rules[i]
	}
	return m
}()

// enabledRules 编译当前设置启用的规则子集。
// 参数 cats：启用的类别列表（空 = 用默认集）。
// 返回 []*rule：启用的规则；类别非法时忽略该类别。
func enabledRules(cats []string) []*rule {
	if len(cats) == 0 {
		cats = DefaultCategories()
	}
	out := make([]*rule, 0, len(cats))
	for _, c := range cats {
		if r, ok := ruleByID["resp-"+c]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Applies 判断本请求路径是否应扫描。
// 参数 s：当前设置；参数 path：请求路径。
// 返回 bool：未启用返回 false；命中排除前缀返回 false；其余 true。
func Applies(s *Settings, path string) bool {
	if s == nil || !s.Enabled {
		return false
	}
	for _, p := range s.ExcludePaths {
		if p != "" && strings.HasPrefix(path, p) {
			return false
		}
	}
	return true
}

// textTypes 需要扫描的响应 Content-Type 前缀/子串（小写比较）。
var textTypes = []string{
	"text/html", "text/plain", "application/json", "application/javascript",
	"application/xml", "text/xml",
}

// Transformable 判断响应是否值得扫描。
// 参数 status：响应状态码；参数 header：响应头。
// 返回 bool：2xx 且 Content-Type 是文本类（或 +json/+xml 后缀）返回 true。
func Transformable(status int, header map[string][]string) bool {
	if status < 200 || status >= 300 {
		return false
	}
	ct := ""
	for k, vs := range header {
		if strings.EqualFold(k, "Content-Type") && len(vs) > 0 {
			ct = strings.ToLower(vs[0])
			break
		}
	}
	if ct == "" {
		return false
	}
	for _, t := range textTypes {
		if strings.HasPrefix(ct, t) {
			return true
		}
	}
	return strings.HasSuffix(ct, "+json") || strings.HasSuffix(ct, "+xml")
}

// Apply 扫描（并按模式改写）响应体。
// 参数 s：当前设置；参数 status：响应状态码；参数 header：响应头；参数 body：明文响应体。
// 返回 ([]byte, detect.Result)：改写后的响应体与归因结果；不适用时原样返回、Result 为空。
//
// mask 模式下逐规则顺序替换；observe 模式只统计命中，body 原样返回。
// 打码后的文本（如 138****1234）不会再匹配任何规则，顺序替换不会二次破坏。
func Apply(s *Settings, status int, header map[string][]string, body []byte) ([]byte, detect.Result) {
	res := detect.Result{Hits: []detect.Hit{}, Categories: []string{}, RuleIDs: []string{}}
	if !Transformable(status, header) || len(body) == 0 {
		return body, res
	}
	maskMode := s.Mode == "mask"
	anyChanged := false
	for _, r := range enabledRules(s.Categories) {
		idx := r.re.FindAllSubmatchIndex(body, -1)
		if idx == nil {
			continue
		}
		res.Hits = append(res.Hits, detect.Hit{
			RuleID: r.id, Name: "响应敏感信息(" + r.category + ")",
			Category: "sensitive", Score: 0, Target: "response",
			Evidence: "", // 证据本身就是敏感内容，事件里不能留原文
		})
		res.RuleIDs = append(res.RuleIDs, r.id)
		res.Categories = appendUnique(res.Categories, r.category)
		if !maskMode {
			continue
		}
		anyChanged = true
		var out bytes.Buffer
		out.Grow(len(body))
		last := 0
		for _, m := range idx {
			out.Write(body[last:m[0]])
			out.Write(r.mask(submatchBytes(body, m)))
			last = m[1]
		}
		out.Write(body[last:])
		body = out.Bytes()
	}
	if anyChanged {
		res.Action = "log"
	}
	return body, res
}

// submatchBytes 把 FindAllSubmatchIndex 的下标切片还原成字节切片。
// 参数 b：原始字节；参数 m：单次匹配的下标切片（成对出现）。
// 返回 [][]byte：各分组的字节切片。
func submatchBytes(b []byte, m []int) [][]byte {
	out := make([][]byte, 0, len(m)/2)
	for i := 0; i+1 < len(m); i += 2 {
		out = append(out, b[m[i]:m[i+1]])
	}
	return out
}

// appendUnique 分类去重，与 detect 包内逻辑一致（本包不导出，避免交叉依赖）。
func appendUnique(list []string, v string) []string {
	for _, item := range list {
		if item == v {
			return list
		}
	}
	return append(list, v)
}
