package detect

// 本文件实现上传文件与反序列化载荷的特征检测。
//
// 与规则引擎的分工：规则引擎靠正则匹配请求文本，适合通用攻击特征；
// 上传检测需要解析 multipart、校验文件名与文件头魔数，反序列化检测需要
// 识别二进制协议标记（Java/PHP/Python/.NET），这些都放进本文件，
// 由数据面在规则检测之后合并进同一份 Result，统一按分数处置。

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"regexp"
	"strings"
)

// 上传/反序列化检测的规则标识，统一前缀便于面板区分来源。
const (
	UploadRuleExt       = "upload-dangerous-ext"  // 危险脚本/可执行扩展名
	UploadRuleDoubleExt = "upload-double-ext"     // 双扩展名绕过（shell.php.jpg）
	UploadRuleBadName   = "upload-bad-filename"   // 文件名包含空字节/路径穿越
	UploadRuleMismatch  = "upload-type-mismatch"  // 声明的图片类型与魔数不符
	UploadRuleScript    = "upload-script-content" // 文件内容包含脚本/可执行标记
	DeserJava           = "deser-java"            // Java 原生序列化
	DeserDotNet         = "deser-dotnet"          // .NET BinaryFormatter
	DeserPickle         = "deser-pickle"          // Python pickle
	DeserPHP            = "deser-php"             // PHP 序列化对象
	DeserAutoType       = "deser-autotype"        // Fastjson/Jackson 自动类型
	DeserYAML           = "deser-yaml-unsafe"     // YAML 不安全标签
	DeserPrototype      = "deser-prototype"       // Node 原型链污染
)

// 危险扩展名集合：脚本与可执行文件一律不允许通过上传接口落盘。
var dangerousExts = map[string]int{
	"php": 70, "php3": 70, "php4": 70, "php5": 70, "php7": 70, "phtml": 70,
	"phar": 70, "jsp": 70, "jspx": 70, "jspf": 70, "asp": 70, "aspx": 70,
	"ashx": 70, "asmx": 70, "cer": 70, "cgi": 70, "pl": 60, "py": 60,
	"rb": 60, "sh": 70, "bash": 70, "exe": 70, "dll": 70, "bat": 70,
	"cmd": 70, "com": 70, "vbs": 70, "ps1": 70, "hta": 70, "jar": 60,
	"war": 60, "class": 60, "svg": 40, "html": 40, "htm": 40, "shtml": 50,
}

// 图片扩展名到魔数校验函数：声明是图片但内容不是图片时告警。
var imageExts = map[string]struct{}{
	"jpg": {}, "jpeg": {}, "png": {}, "gif": {}, "webp": {}, "bmp": {}, "ico": {},
}

// 可执行/脚本内容标记。只扫描文件头 8KB，避免大文件拖慢热路径。
var scriptMarkers = [][]byte{
	[]byte("<?php"), []byte("<?="), []byte("<%"), []byte("<script"),
	[]byte("#!/"), []byte("eval("), []byte("base64_decode("), []byte("shell_exec("),
	[]byte("passthru("), []byte("system("), []byte("assert($_"),
}

// 序列化载荷的正则特征。
var (
	phpSerializedRe = regexp.MustCompile(`(?:^|[\s;&])(?:O:\d+:"|a:\d+:\{|s:\d+:")`)
	autoTypeRe      = regexp.MustCompile(`(?i)"@type"\s*:\s*"[^"]*(?:java\.|javax\.|com\.sun\.|org\.apache\.|ysoserial|jndi)`)
	autoTypeLooseRe = regexp.MustCompile(`(?i)@type\s*[:=]\s*['"]?(?:java|javax|com\.sun)\.`)
)

// InspectUpload 检查 multipart 上传中的文件名与文件内容。
// 参数 contentType：请求的 Content-Type（含 boundary）；参数 body：已读取的请求体。
// 返回 Result：仅填充 Score/Hits/Categories，动作由调用方按全局阈值统一判定。
func InspectUpload(contentType string, body []byte) Result {
	var out Result
	if len(body) == 0 {
		return out
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return out
	}
	boundary := params["boundary"]
	if boundary == "" {
		return out
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	parts := 0
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		parts++
		if parts > 16 {
			_ = part.Close()
			break
		}
		filename := part.FileName()
		if filename == "" {
			_ = part.Close()
			continue
		}
		head := make([]byte, 8192)
		n, _ := part.Read(head)
		_ = part.Close()
		head = head[:n]
		inspectFilename(&out, filename, part.Header.Get("Content-Type"), head)
	}
	return out
}

// inspectFilename 检查单个上传文件的文件名与内容。
// 参数 out：结果累加目标；filename：原始文件名；declared：声明的 Content-Type；head：文件头字节。
func inspectFilename(out *Result, filename, declared string, head []byte) {
	lower := strings.ToLower(strings.TrimSpace(filename))
	base := lower
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}

	// 1) 文件名本身：空字节、路径穿越、控制字符
	if strings.ContainsAny(filename, "\x00\\") || strings.Contains(filename, "..") ||
		strings.Contains(lower, "%00") || hasControl(filename) {
		addHit(out, UploadRuleBadName, "上传文件名包含空字节/路径穿越/控制字符", "upload", 60, "filename", filename)
	}

	// 2) 危险扩展名与双扩展名
	ext := ""
	if i := strings.LastIndexByte(base, '.'); i >= 0 {
		ext = base[i+1:]
	}
	if score, bad := dangerousExts[ext]; bad {
		addHit(out, UploadRuleExt, "上传了可执行/脚本文件（."+ext+"）", "upload", score, "filename", filename)
	}
	if parts := strings.Split(base, "."); len(parts) >= 3 {
		last := parts[len(parts)-1]
		prev := parts[len(parts)-2]
		if _, ok := imageExts[last]; ok {
			if score, bad := dangerousExts[prev]; bad {
				addHit(out, UploadRuleDoubleExt, "双扩展名绕过（."+prev+"."+last+"）", "upload", score, "filename", filename)
			}
		}
	}

	// 3) 内容与扩展名不符：声明图片但魔数不是图片
	if _, isImage := imageExts[ext]; isImage && len(head) > 0 && !looksLikeImage(head) {
		addHit(out, UploadRuleMismatch, "文件内容与图片扩展名不符", "upload", 50, "filename", filename)
	}

	// 4) 文件头中的脚本/可执行标记
	if len(head) > 0 {
		scan := head
		if len(scan) > 2048 {
			scan = scan[:2048]
		}
		lowerHead := bytes.ToLower(scan)
		for _, marker := range scriptMarkers {
			if bytes.Contains(lowerHead, marker) {
				addHit(out, UploadRuleScript, "上传文件包含脚本/可执行内容标记", "upload", 70, "filename", filename)
				break
			}
		}
		if isExecutableHead(head) {
			addHit(out, UploadRuleScript, "上传文件是可执行文件（ELF/MZ/Class）", "upload", 80, "filename", filename)
		}
	}

	// 5) 声明 Content-Type 与实际内容不符（例如声明 image/png 实为 PHP）
	if declared != "" {
		mt := strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
		if strings.HasPrefix(mt, "image/") && len(head) > 0 && !looksLikeImage(head) {
			addHit(out, UploadRuleMismatch, "Content-Type 声明为图片但内容不是图片", "upload", 50, "filename", filename)
		}
	}
}

// InspectDeserialization 检查请求体中的反序列化载荷特征。
// 参数 contentType：请求 Content-Type；参数 body：请求体。
// 返回 Result：仅填充 Score/Hits/Categories；无命中时返回零值。
func InspectDeserialization(contentType string, body []byte) Result {
	var out Result
	if len(body) == 0 {
		return out
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	text := string(body)
	prefix := body
	if len(prefix) > 4096 {
		prefix = prefix[:4096]
	}

	// 1) Java 原生序列化：二进制魔数 ac ed 00 05，或 base64 前缀 rO0AB
	if len(body) >= 4 && body[0] == 0xAC && body[1] == 0xED && body[2] == 0x00 && body[3] == 0x05 {
		addHit(&out, DeserJava, "Java 原生序列化数据流", "deserialization", 85, "body", "AC ED 00 05")
	} else if strings.Contains(text, "rO0AB") || strings.Contains(text, "aced0005") {
		addHit(&out, DeserJava, "Java 序列化数据的 Base64/十六进制特征", "deserialization", 70, "body", "rO0AB")
	}

	// 2) .NET BinaryFormatter：00 01 00 00 00 ff ff ff ff
	if len(body) >= 9 && bytes.Equal(body[:9], []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}) {
		addHit(&out, DeserDotNet, ".NET BinaryFormatter 序列化数据流", "deserialization", 85, "body", "00 01 00 00 00 FF FF FF FF")
	}

	// 3) Python pickle：协议头 0x80 + 版本号，或常见全局函数标记
	if len(body) >= 2 && body[0] == 0x80 && body[1] >= 0x02 && body[1] <= 0x05 {
		addHit(&out, DeserPickle, "Python pickle 序列化数据流", "deserialization", 85, "body", fmt.Sprintf("80 0%d", body[1]))
	} else if bytes.HasPrefix(prefix, []byte("c__builtin__")) || bytes.HasPrefix(prefix, []byte("cos\nsystem")) ||
		bytes.HasPrefix(prefix, []byte("gASV")) || strings.Contains(text, "csubprocess\nPopen") {
		addHit(&out, DeserPickle, "Python pickle 危险全局函数特征", "deserialization", 75, "body", "pickle")
	}

	// 4) PHP 序列化对象：O:数字:"、a:数字:{、s:数字:"
	if phpSerializedRe.Match(prefix) {
		addHit(&out, DeserPHP, "PHP 序列化对象特征", "deserialization", 60, "body", "O:/a:/s:")
	}

	// 5) Fastjson/Jackson 自动类型：@type 指向 JDK/中间件类
	if autoTypeRe.Match(prefix) {
		addHit(&out, DeserAutoType, "JSON 自动类型（@type）指向敏感类", "deserialization", 75, "body", "@type")
	}
	if strings.Contains(ct, "json") && !strings.Contains(text, `"@type"`) && autoTypeLooseRe.Match(prefix) {
		// 兼容不带引号的 @type 变体
		addHit(&out, DeserAutoType, "JSON 自动类型（@type）指向敏感类", "deserialization", 75, "body", "@type")
	}

	// 6) YAML 不安全标签
	if bytes.Contains(prefix, []byte("!!python/object")) || bytes.Contains(prefix, []byte("!!javax.script")) ||
		bytes.Contains(prefix, []byte("!!com.sun.")) {
		addHit(&out, DeserYAML, "YAML 不安全反序列化标签", "deserialization", 70, "body", "!!")
	}

	// 7) Node 原型链污染：仅在 JSON/表单请求里检查，降低误报
	if (strings.Contains(ct, "json") || strings.Contains(ct, "x-www-form-urlencoded")) &&
		(bytes.Contains(prefix, []byte("__proto__")) || bytes.Contains(prefix, []byte("constructor.prototype"))) {
		addHit(&out, DeserPrototype, "Node 原型链污染载荷（__proto__）", "deserialization", 50, "body", "__proto__")
	}

	// 8) 常见 ysoserial / JNDI 利用链关键字（仅在 JSON/序列化上下文中）
	if strings.Contains(ct, "json") || strings.Contains(ct, "serialized") {
		for _, kw := range []string{"JdbcRowSetImpl", "TemplatesImpl", "CommonsBeanutils", "jndi:ldap", "JNDIExploit"} {
			if strings.Contains(text, kw) {
				addHit(&out, DeserJava, "疑似 Java 反序列化利用链："+kw, "deserialization", 80, "body", kw)
				break
			}
		}
	}

	// 9) 额外的 Base64 编码 pickle/Java 载荷（参数里常见）
	if len(body) < 2048 {
		for _, token := range []string{"gASV", "gAJ9", "rO0ABXNy"} {
			if strings.Contains(text, token) {
				addHit(&out, DeserPickle, "Base64 编码的序列化载荷特征", "deserialization", 65, "body", token)
				break
			}
		}
	}
	return out
}

// InspectPayload 统一执行上传与反序列化检查，由数据面一次调用。
func InspectPayload(contentType string, body []byte) Result {
	var out Result
	if len(body) == 0 {
		return out
	}
	up := InspectUpload(contentType, body)
	deser := InspectDeserialization(contentType, body)
	mergeInto(&out, &up)
	mergeInto(&out, &deser)
	return out
}

// addHit 追加一条命中并同步类别/规则 ID。
func addHit(out *Result, ruleID, name, category string, score int, target, evidence string) {
	out.Score += score
	out.Hits = append(out.Hits, Hit{
		RuleID: ruleID, Name: name, Category: category, Score: score,
		Target: target, Evidence: truncateEvidence(evidence),
	})
	out.RuleIDs = appendUnique(out.RuleIDs, ruleID)
	out.Categories = appendUnique(out.Categories, category)
}

// mergeInto 把 src 合并进 dst（分数、命中、类别、规则 ID）。
func mergeInto(dst, src *Result) {
	if src == nil || len(src.Hits) == 0 {
		return
	}
	dst.Score += src.Score
	dst.Hits = append(dst.Hits, src.Hits...)
	for _, id := range src.RuleIDs {
		dst.RuleIDs = appendUnique(dst.RuleIDs, id)
	}
	for _, c := range src.Categories {
		dst.Categories = appendUnique(dst.Categories, c)
	}
}

// looksLikeImage 判断文件头是否为常见图片格式。
func looksLikeImage(head []byte) bool {
	if len(head) < 3 {
		return false
	}
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}): // JPEG
		return true
	case bytes.HasPrefix(head, []byte{0x89, 0x50, 0x4E, 0x47}): // PNG
		return true
	case bytes.HasPrefix(head, []byte("GIF8")): // GIF
		return true
	case len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return true
	case bytes.HasPrefix(head, []byte("BM")): // BMP
		return true
	case bytes.HasPrefix(head, []byte{0x00, 0x00, 0x01, 0x00}): // ICO
		return true
	}
	return false
}

// isExecutableHead 判断文件头是否为 ELF / PE / Java Class。
func isExecutableHead(head []byte) bool {
	return bytes.HasPrefix(head, []byte{0x7F, 'E', 'L', 'F'}) || // ELF
		bytes.HasPrefix(head, []byte("MZ")) || // PE
		bytes.HasPrefix(head, []byte{0xCA, 0xFE, 0xBA, 0xBE}) // Java Class
}

// hasControl 判断字符串是否包含 ASCII 控制字符（排除常见空白）。
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// truncateEvidence 截断证据片段，避免事件表被超长文件名/载荷撑爆。
func truncateEvidence(s string) string {
	const max = 160
	if len(s) <= max {
		return s
	}
	return s[:max]
}
