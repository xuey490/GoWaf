// Package detect 实现规则引擎：归一化 → AC 预筛 → 正则匹配 → 评分。
//
// 归一化是绕过防御的最大敌人。攻击者可以用 %2527、%u0027、&#39;、
// 全角字符、SQL 注释、大小写混写来绕过简单关键字匹配。这里统一处理一遍，
// 让后续规则只面对最"干净"的形态。
package detect

import (
	"html"
	"net/url"
	"strings"
	"unicode"
)

// maxDecodeRounds 多层编码最多解几层（防止构造超深编码拖死 CPU）。
const maxDecodeRounds = 3

// Normalize 返回归一化后的字符串。
func Normalize(raw string) string {
	if raw == "" {
		return ""
	}
	s := raw

	// 1) 多层 URL 编码
	for i := 0; i < maxDecodeRounds; i++ {
		decoded, err := url.QueryUnescape(s)
		if err != nil || decoded == s {
			break
		}
		s = decoded
	}

	// 2) HTML 实体（&#39; / &lt; / &quot;）
	if strings.Contains(s, "&") {
		s = html.UnescapeString(s)
	}

	// 3) 全角转半角
	s = toHalfWidth(s)

	// 4) 去 SQL 注释与 MySQL 版本注释 /*!50000 ... */
	s = stripSQLComment(s)

	// 5) 空白压缩（保留单个空格，避免 "un ion" 这类拆分绕过）
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) || r == 0 {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}

	return strings.TrimSpace(strings.ToLower(b.String()))
}

// toHalfWidth 全角字符转半角（含全角空格）。
func toHalfWidth(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0x3000:
			b.WriteRune(' ')
		case r >= 0xFF01 && r <= 0xFF5E:
			b.WriteRune(r - 0xFEE0)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripSQLComment 去掉 /* */ 与 -- 行注释，防止 /*!50000union*/ 绕过。
func stripSQLComment(s string) string {
	if !strings.Contains(s, "/*") && !strings.Contains(s, "--") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	depth := 0
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if depth > 0 {
			if i+1 < len(s) && s[i] == '*' && s[i+1] == '/' {
				depth--
				i += 2
				continue
			}
			i++
			continue
		}
		if i+1 < len(s) && s[i] == '-' && s[i+1] == '-' {
			// 行注释：跳到换行
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				break
			}
			i += j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// IsStaticPath 判断静态资源，静态资源不做内容检测（性能关键）。
func IsStaticPath(path string) bool {
	idx := strings.LastIndexByte(path, '.')
	if idx < 0 || idx == len(path)-1 {
		return false
	}
	ext := strings.ToLower(path[idx:])
	switch ext {
	case ".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp",
		".woff", ".woff2", ".ttf", ".eot", ".ico", ".map", ".mp4", ".webm":
		return true
	}
	return false
}

// sensitiveAssetTokens 是「看着像静态资源、实则是配置/凭据」的文件名词干。
//
// 攻击者不会只探 /.env，还会探 /env.js、/runtime-config.js、/aws-exports.js
// 这类前端运行时配置——它们的扩展名正好落在 IsStaticPath 的静态列表里，
// 如果直接按扩展名跳过检测，这些探测就完全绕过 WAF（实测日志中大量出现）。
var sensitiveAssetTokens = map[string]struct{}{
	"env": {}, "config": {}, "configuration": {}, "settings": {}, "setting": {},
	"credential": {}, "credentials": {}, "secret": {}, "secrets": {},
	"auth": {}, "aws": {}, "gcp": {}, "azure": {},
	"git": {}, "docker": {}, "kube": {}, "ssh": {},
	"npmrc": {}, "netrc": {}, "claude": {}, "cursor": {},
	"appsettings": {}, "installed": {}, "manifest": {},
}

// IsSensitiveAssetPath 判断静态资源路径是否其实是配置/凭据类文件。
// 只看文件名主干（去扩展名后按 - _ . 切词），避免 /config/logo.png 这类误判。
func IsSensitiveAssetPath(p string) bool {
	if p == "" {
		return false
	}
	name := p
	if idx := strings.LastIndexByte(p, '/'); idx >= 0 {
		name = p[idx+1:]
	}
	if name == "" {
		return false
	}
	// 去掉扩展名（支持 a.ts.js 这种多重后缀，只取第一段）
	if dot := strings.IndexByte(name, '.'); dot > 0 {
		name = name[:dot]
	}
	name = strings.ToLower(name)
	for _, tok := range strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	}) {
		if _, ok := sensitiveAssetTokens[tok]; ok {
			return true
		}
	}
	return false
}

// Truncate 截断到 n 字节，避免日志与检测被超长输入拖死。
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
