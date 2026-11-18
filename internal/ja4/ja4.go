// Package ja4 实现 TLS ClientHello 指纹（JA4）的采集与识别。
//
// JA4 是 FoxIO 公开的客户端指纹算法（https://github.com/FoxIO-LLC/ja4），
// 形如 t13d1516h2_8daaf6152771_e5627efa2ab1：第一段是「协议+TLS 版本+SNI 形态+
// 密码套件数+扩展数+首个 ALPN」，第二段是密码套件列表的截断 SHA-256，
// 第三段是扩展列表 + 签名算法列表的截断 SHA-256。
//
// 采集方式：包装 TLS 监听器，在把连接交给 crypto/tls 之前偷看首个 ClientHello
// 记录并计算指纹，随后按连接四元组（远端地址:端口）登记进进程级表格；
// 数据面通过 ja4.Lookup(r.RemoteAddr) 取回本连接的指纹。HTTP/1.1 与 HTTP/2
// 的请求 RemoteAddr 都是底层连接的远端地址，同一连接上的所有请求拿到同一指纹。
//
// 诚实声明：指纹与表项按连接四元组关联，连接闲置期间表项常驻（每连接一条，
// 内存可忽略）；代理链路（CDN 回源）下看到的是代理侧 ClientHello，此时指纹
// 只能区分接入链路而非最终访客。
package ja4

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// isGREASE 判断一个 16 位值是否为 GREASE 占位值（RFC 8701）。
// GREASE 规则：低 8 位与高 8 位相同且 nibble 为 0xa，如 0x0a0a、0x1a1a。
// 参数 v：密码套件 / 扩展 / 版本等 16 位值。
// 返回 bool：是 GREASE 返回 true，JA4 算法要求全流程忽略 GREASE。
func isGREASE(v uint16) bool { return v&0x0f0f == 0x0a0a }

// versionName 把 TLS 版本号转成 JA4 的两位版本段。
// 参数 v：16 位版本号（0x0304 等）。
// 返回 string："13"/"12"/"11"/"10"/"s3"/"s2"/"d1"/"d2"/"d3"；未知版本返回 "00"。
func versionName(v uint16) string {
	switch v {
	case 0x0304:
		return "13"
	case 0x0303:
		return "12"
	case 0x0302:
		return "11"
	case 0x0301:
		return "10"
	case 0x0300:
		return "s3"
	case 0x0002:
		return "s2"
	case 0xfeff:
		return "d1"
	case 0xfefd:
		return "d2"
	case 0xfefc:
		return "d3"
	}
	return "00"
}

// alpnSeg 取首个 ALPN 值的两位指纹段：取首尾各一个 ASCII 字母数字字符；
// 非字母数字时取十六进制表示的首尾字符；无 ALPN 返回 "00"。
// 参数 raw：首个 ALPN 协议名原始字节。
// 返回 string：两位指纹段。
func alpnSeg(raw []byte) string {
	if len(raw) == 0 {
		return "00"
	}
	isAlnum := func(b byte) bool {
		return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
	}
	first, last := raw[0], raw[len(raw)-1]
	if isAlnum(first) && isAlnum(last) {
		return string([]byte{first, last})
	}
	// 规范规定：首尾字节含非字母数字时，改用完整十六进制表示的首尾字符
	h := hex.EncodeToString(raw)
	if len(h) < 2 {
		return "00"
	}
	return h[:1] + h[len(h)-1:]
}

// hash12 计算字符串的 SHA-256 并截断前 12 个十六进制字符。
// 参数 s：待哈希串。返回 string：12 字符小写十六进制。
func hash12(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// hello 是 ClientHello 解析结果（已剔除 GREASE）。
type hello struct {
	Version   uint16   // ClientHello 顶层 protocol version
	Versions  []uint16 // supported_versions 扩展声明的版本列表（可为空）
	Ciphers   []uint16 // 密码套件列表
	Exts      []uint16 // 扩展类型列表
	SigAlgs   []uint16 // signature_algorithms 内容（按出现顺序）
	HasSNI    bool     // 是否携带 SNI 扩展
	ALPNFirst []byte   // 首个 ALPN 协议名原始字节（nil 表示无）
}

// parseHello 解析一条完整的 TLS record 载荷（应为 ClientHello）。
// 参数 rec：首个 TLS record 的载荷字节。
// 返回 (*hello, bool)：解析成功返回结果与 true；非握手 / 非 ClientHello / 结构
// 截断时返回 false（此时无法计算指纹）。
func parseHello(rec []byte) (*hello, bool) {
	if len(rec) < 4 || rec[0] != 0x01 { // handshake type 必须是 client_hello
		return nil, false
	}
	body := rec[4:]
	need := func(n int) bool { return len(body) >= n }
	// 结构：version(2) random(32) session_id(1+N) ciphers(2+N) compression(1+N) [exts]
	if !need(35) {
		return nil, false
	}
	h := &hello{}
	h.Version = binary.BigEndian.Uint16(body[:2])
	p := 34
	sidLen := int(body[p])
	p++
	if !need(p + sidLen) {
		return nil, false
	}
	p += sidLen
	if !need(p + 2) {
		return nil, false
	}
	csLen := int(binary.BigEndian.Uint16(body[p : p+2]))
	p += 2
	if !need(p+csLen) || csLen%2 != 0 {
		return nil, false
	}
	for i := 0; i+1 < csLen; i += 2 {
		v := binary.BigEndian.Uint16(body[p+i : p+i+2])
		if !isGREASE(v) {
			h.Ciphers = append(h.Ciphers, v)
		}
	}
	p += csLen
	if !need(p + 1) {
		return nil, false
	}
	compLen := int(body[p])
	p++
	if !need(p + compLen) {
		return nil, false
	}
	p += compLen
	// 扩展区可选（TLS 1.0 老客户端可能没有）
	if !need(p + 2) {
		return h, true
	}
	extLen := int(binary.BigEndian.Uint16(body[p : p+2]))
	p += 2
	if !need(p + extLen) {
		return nil, false
	}
	exts := body[p : p+extLen]
	for i := 0; i+4 <= len(exts); {
		typ := binary.BigEndian.Uint16(exts[i : i+2])
		l := int(binary.BigEndian.Uint16(exts[i+2 : i+4]))
		if i+4+l > len(exts) {
			return nil, false
		}
		data := exts[i+4 : i+4+l]
		if !isGREASE(typ) {
			h.Exts = append(h.Exts, typ)
		}
		switch typ {
		case 0x0000: // server_name
			h.HasSNI = true
		case 0x000d: // signature_algorithms：列表长度(2) + 每项 2 字节
			if len(data) >= 2 {
				n := int(binary.BigEndian.Uint16(data[:2]))
				if 2+n <= len(data) {
					for j := 2; j+1 < 2+n; j += 2 {
						h.SigAlgs = append(h.SigAlgs, binary.BigEndian.Uint16(data[j:j+2]))
					}
				}
			}
		case 0x0010: // ALPN：列表长度(2) + [名长度(1)+名]
			if len(data) >= 3 {
				n := int(binary.BigEndian.Uint16(data[:2]))
				if n >= 1 && 2+n <= len(data) {
					ln := int(data[2])
					if ln > 0 && 3+ln <= len(data) {
						h.ALPNFirst = append([]byte(nil), data[3:3+ln]...)
					}
				}
			}
		case 0x002b: // supported_versions：列表长度(1) + 每项 2 字节
			if len(data) >= 1 {
				n := int(data[0])
				if 1+n <= len(data) {
					for j := 1; j+1 < 1+n; j += 2 {
						v := binary.BigEndian.Uint16(data[j : j+2])
						if !isGREASE(v) {
							h.Versions = append(h.Versions, v)
						}
					}
				}
			}
		}
		i += 4 + l
	}
	return h, true
}

// FromRecord 由首个 TLS record 载荷计算 JA4 指纹。
// 参数 rec：TLS record 载荷（含 ClientHello）。
// 返回 string：JA4 指纹；解析失败返回空串（调用方按无指纹处理）。
func FromRecord(rec []byte) string {
	h, ok := parseHello(rec)
	if !ok || len(h.Ciphers) == 0 {
		return ""
	}

	// 版本段：优先 supported_versions 的最高版本，回退顶层 protocol version
	ver := h.Version
	for _, v := range h.Versions {
		if v > ver {
			ver = v
		}
	}

	// 密码套件哈希：4 位小写十六进制、按 hex 字符串排序、逗号分隔、SHA-256 截 12
	cipherHex := make([]string, 0, len(h.Ciphers))
	for _, c := range h.Ciphers {
		cipherHex = append(cipherHex, hex.EncodeToString([]byte{byte(c >> 8), byte(c)}))
	}
	sortStrings(cipherHex)
	cipherPart := "000000000000"
	if len(cipherHex) > 0 {
		cipherPart = hash12(strings.Join(cipherHex, ","))
	}

	// 扩展哈希：剔除 SNI(0000) 与 ALPN(0010)，排序后拼接；签名算法按出现顺序
	// 用 "_" 接在后面（一起参与哈希）
	extHex := make([]string, 0, len(h.Exts))
	for _, e := range h.Exts {
		if e == 0x0000 || e == 0x0010 {
			continue
		}
		extHex = append(extHex, hex.EncodeToString([]byte{byte(e >> 8), byte(e)}))
	}
	sortStrings(extHex)
	var extStr string
	if len(extHex) > 0 {
		extStr = strings.Join(extHex, ",")
	}
	sigHex := make([]string, 0, len(h.SigAlgs))
	for _, s := range h.SigAlgs {
		sigHex = append(sigHex, hex.EncodeToString([]byte{byte(s >> 8), byte(s)}))
	}
	if len(sigHex) > 0 {
		extStr += "_" + strings.Join(sigHex, ",")
	}
	extPart := "000000000000"
	if extStr != "" {
		extPart = hash12(extStr)
	}

	// 第一段：t + 版本 + SNI 形态 + 密码数 + 扩展数 + ALPN 段
	sni := "i"
	if h.HasSNI {
		sni = "d"
	}
	csCount := len(h.Ciphers)
	if csCount > 99 {
		csCount = 99
	}
	extCount := len(h.Exts)
	if extCount > 99 {
		extCount = 99
	}
	var b strings.Builder
	b.WriteByte('t')
	b.WriteString(versionName(ver))
	b.WriteString(sni)
	b.WriteString(pad2(csCount))
	b.WriteString(pad2(extCount))
	b.WriteString(alpnSeg(h.ALPNFirst))
	return b.String() + "_" + cipherPart + "_" + extPart
}

// pad2 把非负整数格式化成两位十进制（超 99 截断到 99，负数按 0）。
func pad2(n int) string {
	if n < 0 {
		n = 0
	}
	if n > 99 {
		n = 99
	}
	const digits = "0123456789"
	return string([]byte{digits[n/10], digits[n%10]})
}

// sortStrings 原地升序排序（标准 sort.Slice 的零依赖替代，数据量极小）。
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
