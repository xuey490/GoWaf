package detect

// 检测前多层解码：补归一化覆盖不到的绕过面。
//
// Normalize 已经处理了多层 URL 解码、HTML 实体、全角字符与 SQL 注释，
// 但攻击者还有三类"检测器看不懂原文"的绕过面：
//
//	1. base64：payload 整体 base64 编码（?token=JyUyME9SJyUyMSc=），
//	   原文里根本没有攻击特征，解一层之后才是 URL 编码的 SQLi；
//	2. 压缩请求体：API 客户端 / 攻击脚本常带 Content-Encoding: gzip 的请求体，
//	   原文是二进制压缩流，任何正则都扫不到；
//	3. Unicode 转义：JSON 体内的 \u0027（' 的转义）是合法 JSON 写法，
//	   原文层面看不见引号。
//
// 本文件对 query 与 body 生成「解码变体」：变体不替换原文，而是作为附加
// 检测文本并入扫描（原文照检，命中去重按规则 ID，不会重复计分）。
// 变体同时喂给自研规则引擎与 libinjection 语义通道；CRS 不吃变体——
// Coraza 自带 t:urlDecode / t:base64Decode 等变换链，再喂会双重解码。
//
// 解码的防御性约束（防 DoS）：
//   - 只在快速特征检查通过时才尝试（base64 字符集/长度、压缩魔数、含 \u），
//     不对每个请求盲目解；
//   - 单变体字节上限 MaxBytes，解码层数上限 MaxDepth；
//   - 解码失败一律静默放弃（变体少一个不会漏检原文）。

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 解码配置的缺省值（设置项缺省时的兜底）。
const (
	DefaultDecodeEnabled = true
	DefaultDecodeDepth   = 2 // base64 最多解 2 层（base64 of base64）
	DefaultDecodeBytes   = 64 * 1024
)

// DecodeConfig 解码变体的配置，Reload 时编译成快照。
type DecodeConfig struct {
	Enabled  bool // 总开关
	MaxDepth int  // base64 最大解层层深（1~3）
	MaxBytes int  // 单变体字节上限，超过即放弃该变体
}

// DefaultDecodeConfig 返回默认配置（启用，深度 2，64KB）。
func DefaultDecodeConfig() DecodeConfig {
	return DecodeConfig{Enabled: DefaultDecodeEnabled, MaxDepth: DefaultDecodeDepth, MaxBytes: DefaultDecodeBytes}
}

// DecodeVariants 生成 query 与 body 的解码变体。
// 参数 query：原始查询串；参数 body：原始请求体文本；参数 cfg：解码配置。
// 返回 ([]string, []string)：query 变体与 body 变体（可能为空，不代表失败）。
// 原文不包含在返回值里——调用方把变体作为附加检测文本并入扫描。
func DecodeVariants(query, body string, cfg DecodeConfig) (qvars, bvars []string) {
	if !cfg.Enabled {
		return nil, nil
	}
	depth := cfg.MaxDepth
	if depth < 1 || depth > 3 {
		depth = DefaultDecodeDepth
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultDecodeBytes
	}

	if query != "" {
		// query 是整段 RawQuery（key=value&...），base64 通常编码的是单个参数值。
		// 按 & 分割逐段尝试：值是 base64 且解出文本时，产出 "k=decoded" 变体
		// （保留 key 上下文，事件里的证据可读）；整段不是键值形态时退回整段解码。
		qvars = decodeQueryVariants(query, depth, maxBytes)
	}
	if body != "" {
		// body 先试压缩解压（原文是二进制流时，文本类解码无意义），再试文本类解码
		if raw, ok := decompressBody([]byte(body), maxBytes); ok {
			bvars = append(bvars, truncateRunes(string(raw), maxBytes))
		} else {
			bvars = append(bvars, decodeTextVariants(body, depth, maxBytes)...)
		}
	}
	return dedupeVariants(qvars), dedupeVariants(bvars)
}

// decodeQueryVariants 对 query 生成解码变体：按 & 分段，对每个参数值做
// base64 / unicode 转义解码，解出文本时产出 "k=decoded" 形式的变体。
// 参数 query：原始 RawQuery；depth：base64 层数上限；maxBytes：单变体上限。
// 返回 []string：变体列表（可能为空）。
func decodeQueryVariants(query string, depth, maxBytes int) []string {
	var out []string
	segments := strings.Split(query, "&")
	for _, seg := range segments {
		eq := strings.IndexByte(seg, '=')
		if eq < 0 {
			continue
		}
		key, val := seg[:eq], seg[eq+1:]
		if len(val) < 16 {
			continue
		}
		for _, decoded := range decodeTextVariants(val, depth, maxBytes) {
			out = append(out, key+"="+decoded)
		}
	}
	return out
}

// decodeTextVariants 对一段文本生成 base64 与 unicode 转义两类变体。
// 参数 s：原文；depth：base64 层数上限；maxBytes：单变体上限。
// 返回 []string：解码出的变体（可能为空）。
func decodeTextVariants(s string, depth, maxBytes int) []string {
	var out []string
	// 1) base64：payload 整体编码是最常见的绕过。解出的内容可能还是 URL 编码，
	//    那一层由引擎内的 Normalize 兜住，这里只负责把 base64 剥掉。
	if cur, ok := tryBase64(s, maxBytes); ok {
		out = append(out, cur)
		for i := 1; i < depth; i++ {
			next, ok2 := tryBase64(cur, maxBytes)
			if !ok2 {
				break
			}
			out = append(out, next)
			cur = next
		}
	}
	// 2) JSON/JS unicode 转义（\u0027）：合法 JSON 写法，原文层面看不见引号。
	if esc, ok := decodeUnicodeEscapes(s, maxBytes); ok {
		out = append(out, esc)
	}
	return out
}

// tryBase64 判断文本是否"像 base64"并尝试解码。
// 判据：长度 ≥16 且为 4 的倍数、字符集在 base64/std 或 base64url 字母表内。
// 参数 s：输入文本；maxBytes：解码结果上限。
// 返回 (string, bool)：解码结果与是否成功；结果必须是通过 UTF-8 合法性检查的文本。
func tryBase64(s string, maxBytes int) (string, bool) {
	if len(s) < 16 || len(s)%4 != 0 || len(s) > maxBytes {
		return "", false
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		if enc == base64.StdEncoding || enc == base64.URLEncoding {
			// 带填充的编码要求长度含填充后为 4 的倍数——上面已保证
			decoded, err := enc.DecodeString(s)
			if err != nil || !plausibleText(decoded) {
				continue
			}
			return truncateRunes(string(decoded), maxBytes), true
		}
		// Raw 变体只在长度不是 4 的倍数时才可能成功，前面已过滤，跳过
	}
	return "", false
}

// plausibleText 粗判字节是否为"值得检测的文本"：UTF-8 合法且无 NUL，
// 可打印比例 ≥ 80%（解出纯随机字节的 base64 串没有检测价值）。
// 参数 b：解码后的字节。
// 返回 bool：通过检查返回 true。
func plausibleText(b []byte) bool {
	if len(b) == 0 || !utf8.Valid(b) {
		return false
	}
	printable := 0
	runes := 0
	for _, r := range string(b) {
		runes++
		if r == 0 {
			return false
		}
		if r >= 0x20 || r == '\n' || r == '\t' || r == '\r' {
			printable++
		}
	}
	// 按 rune 计可打印比例（多字节字符整体算一个 rune）
	return runes > 0 && printable*5 >= runes*4
}

// decodeUnicodeEscapes 解码 JSON/JS 风格的 \uXXXX 转义。
// 参数 s：原文；maxBytes：结果上限。
// 返回 (string, bool)：含 \u 转义且解码成功时返回解码文本与 true。
func decodeUnicodeEscapes(s string, maxBytes int) (string, bool) {
	if !strings.Contains(s, `\u`) || len(s) > maxBytes {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(s))
	wrote := false
	for i := 0; i < len(s); {
		if i+6 <= len(s) && s[i] == '\\' && s[i+1] == 'u' {
			if v, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
				b.WriteRune(rune(v))
				wrote = true
				i += 6
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	if !wrote {
		return "", false
	}
	return b.String(), true
}

// decompressBody 解压压缩的请求体。
// 支持 gzip（魔数 1f 8b）、zlib（0x78 开头）与原始 deflate（前两者都不匹配时兜底尝试）。
// 参数 body：原始请求体字节；maxBytes：解压结果上限（防止 zip 炸弹撑爆内存）。
// 返回 ([]byte, bool)：解压结果与是否成功。
func decompressBody(body []byte, maxBytes int) ([]byte, bool) {
	if len(body) < 8 {
		return nil, false
	}
	limit := int64(maxBytes)
	try := func(newReader func(io.Reader) (io.Reader, error)) ([]byte, bool) {
		r, err := newReader(bytes.NewReader(body))
		if err != nil {
			return nil, false
		}
		out, err := io.ReadAll(io.LimitReader(r, limit+1))
		if err != nil || len(out) == 0 || int64(len(out)) > limit {
			return nil, false
		}
		return out, true
	}
	if bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		if out, ok := try(func(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) }); ok {
			return out, true
		}
	}
	if body[0] == 0x78 {
		if out, ok := try(func(r io.Reader) (io.Reader, error) { return zlib.NewReader(r) }); ok {
			return out, true
		}
	}
	// 原始 deflate：没有魔数，只在文本类解码前兜底试一次
	if out, ok := try(func(r io.Reader) (io.Reader, error) { return flate.NewReader(r), nil }); ok {
		return out, true
	}
	return nil, false
}

// truncateRunes 按 rune 边界截断字符串，防止多字节字符被拦腰截断产生乱码变体。
// 参数 s：原始字符串；max：字节上限。
// 返回 string：长度不超过 max 且不破坏 UTF-8 序列的前缀。
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	end := max
	// 回退到最近的 rune 起点，保证截断处是完整字符
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}

// dedupeVariants 变体去重（嵌套编码可能解出相同文本）。
// 参数 list：变体列表。
// 返回 []string：去重后的非空变体。
func dedupeVariants(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
