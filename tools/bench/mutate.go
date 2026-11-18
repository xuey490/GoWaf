package main

import (
	"encoding/base64"
	"strings"
)

// mutation 是对种子 payload 的确定性变形函数（i 为样本序号，用于确定性地
// 在多种形态间轮换）。所有变形都是"检测器理应穿透或识破"的真实绕过手段——
// 变形后检不出就如实计入漏检，不做任何特殊照顾。
type mutation struct {
	name string
	fn   func(p string, i int) string
}

// mutations 变形清单（与《P1-样本库建设方案.md》3.4 对应）：
// 编码类 5 种、语法类 3 种、结构类 4 种。
var mutations = []mutation{
	{"identity", mutIdentity},
	{"urlenc1", mutURLEnc1},
	{"urlenc2", mutURLEnc2},
	{"encall", mutEncAll},
	{"case", mutCaseMangle},
	{"comment", mutCommentInsert},
	{"wstab", mutWsTab},
	{"wslf", mutWsLF},
	{"entity", mutEntityDec},
	{"uni", mutUniEscape},
	{"b64", mutB64},
	{"quotepad", mutQuotePad},
}

// mutNames 返回变形名列表（报告/生成用）。
func mutNames() []string {
	out := make([]string, len(mutations))
	for i, m := range mutations {
		out[i] = m.name
	}
	return out
}

func mutIdentity(p string, _ int) string { return p }

// mutURLEnc1 单次 URL 编码（QueryEscape：字母数字保留，特殊字符 %xx）。
func mutURLEnc1(p string, _ int) string { return uenc(p) }

// mutURLEnc2 双重 URL 编码（%2527 形态，考验多层解码）。
func mutURLEnc2(p string, _ int) string { return uenc(uenc(p)) }

// mutEncAll 逐字节百分号大写编码（最激进的编码形态）。
func mutEncAll(p string, _ int) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		b.WriteString("%")
		b.WriteString(strings.ToUpper(hex2(p[i]>>4)))
		b.WriteString(hex2(p[i] & 0xF))
	}
	return b.String()
}

func hex2(v byte) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[v]})
}

// mutCaseMangle 大小写交错（SeNsItIvE 混写）。
func mutCaseMangle(p string, _ int) string {
	var b strings.Builder
	upper := false
	for _, r := range p {
		if r >= 'a' && r <= 'z' {
			if upper {
				r = r - 32
			}
			upper = !upper
		} else if r >= 'A' && r <= 'Z' {
			if !upper {
				r = r + 32
			}
			upper = !upper
		}
		b.WriteRune(r)
	}
	return b.String()
}

// mutCommentInsert 在第一个字母前插入 SQL 块注释（un/**/ion 形态），
// 考验 Normalize 的注释剥离。
func mutCommentInsert(p string, i int) string {
	idx := strings.IndexFunc(p, func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' })
	if idx < 0 || idx >= len(p)-1 {
		return p
	}
	tag := "/*x*/"
	if i%2 == 1 {
		tag = "/*%00*/"
	}
	return p[:idx] + tag + p[idx:]
}

// mutWsTab 空白 → %09（tab）。
func mutWsTab(p string, _ int) string { return strings.ReplaceAll(p, " ", "%09") }

// mutWsLF 空白 → %0a（换行）。
func mutWsLF(p string, _ int) string { return strings.ReplaceAll(p, " ", "%0a") }

// mutEntityDec HTML 实体（十进制/十六进制轮换）。
func mutEntityDec(p string, i int) string {
	rep := strings.NewReplacer("<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;", "&", "&amp;")
	if i%2 == 0 {
		rep = strings.NewReplacer("<", "&#x3C;", ">", "&#x3E;", "\"", "&#x22;", "'", "&#x27;", "&", "&#x26;")
	}
	return rep.Replace(p)
}

// mutUniEscape JS/JSON unicode 转义（\u003C 形态）。
func mutUniEscape(p string, _ int) string {
	rep := strings.NewReplacer("<", `\u003C`, ">", `\u003E`, `"`, `\u0022`, "'", `\u0027`, ";", `\u003B`, "&", `\u0026`)
	return rep.Replace(p)
}

// mutB64 整体 base64（对应检测前解码通道的 query 参数 base64）。
func mutB64(p string, _ int) string {
	return base64.StdEncoding.EncodeToString([]byte(p))
}

// mutQuotePad 引号包裹（'payload / "payload，考验闭合类规则）。
func mutQuotePad(p string, i int) string {
	if i%2 == 0 {
		return "'" + p + "'"
	}
	return "\"" + p + "\""
}
