package detect

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

// filePart 描述一个待上传的文件字段。
type filePart struct {
	field       string // 表单字段名
	filename    string // 文件名
	contentType string // 声明的 Content-Type（可空；CreateFormFile 固定写 octet-stream，需自定义时走 CreatePart）
	content     []byte // 文件内容
}

// buildMultipart 构造 multipart 请求体与对应 Content-Type。
func buildMultipart(t *testing.T, files ...filePart) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition",
			fmt.Sprintf(`form-data; name="%s"; filename="%s"`, f.field, f.filename))
		ct := f.contentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		h.Set("Content-Type", ct)
		fw, err := w.CreatePart(h)
		if err != nil {
			t.Fatalf("构造 multipart 失败: %v", err)
		}
		if _, err := fw.Write(f.content); err != nil {
			t.Fatalf("写入文件内容失败: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 multipart 失败: %v", err)
	}
	return w.FormDataContentType(), buf.Bytes()
}

// hasRule 判断结果里是否包含指定规则 ID。
func hasRule(r Result, ruleID string) bool {
	for _, id := range r.RuleIDs {
		if id == ruleID {
			return true
		}
	}
	return false
}

// TestInspectUploadDangerousExt 验证危险扩展名检测。
func TestInspectUploadDangerousExt(t *testing.T) {
	ct, body := buildMultipart(t, filePart{
		field: "file", filename: "shell.php",
		content: []byte("<?php eval($_POST[1]); ?>"),
	})
	res := InspectUpload(ct, body)
	if !hasRule(res, UploadRuleExt) {
		t.Fatalf("应命中危险扩展名规则: %+v", res)
	}
	if !hasRule(res, UploadRuleScript) {
		t.Fatalf("内容含 PHP 代码应命中脚本标记规则: %+v", res)
	}
	if res.Score < 70 {
		t.Fatalf("评分异常: %d", res.Score)
	}
	if len(res.Categories) != 1 || res.Categories[0] != "upload" {
		t.Fatalf("类别异常: %v", res.Categories)
	}
}

// TestInspectUploadDoubleExt 验证双扩展名绕过（shell.php.jpg）。
func TestInspectUploadDoubleExt(t *testing.T) {
	// 内容用真实 JPEG 魔数，避免触发图片不符误报
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x00}, 32)...)
	ct, body := buildMultipart(t, filePart{
		field: "file", filename: "shell.php.jpg", content: jpeg,
	})
	res := InspectUpload(ct, body)
	if !hasRule(res, UploadRuleDoubleExt) {
		t.Fatalf("应命中双扩展名规则: %+v", res)
	}
	// jpg 是合法图片扩展名，不应报内容不符
	if hasRule(res, UploadRuleMismatch) {
		t.Fatalf("JPEG 魔数不该报内容不符: %+v", res)
	}
	// 反向双扩展名：photo.jpg.php 直接算危险扩展名
	ct, body = buildMultipart(t, filePart{
		field: "file", filename: "photo.jpg.php", content: []byte("x"),
	})
	res = InspectUpload(ct, body)
	if !hasRule(res, UploadRuleExt) {
		t.Fatalf(".php 结尾应命中危险扩展名: %+v", res)
	}
}

// TestInspectUploadBadName 验证文件名攻击特征：路径穿越/空字节。
//
// 注意：Go 1.24 起 multipart.Part.FileName() 会先剥离路径部分再返回，
// "../etc/passwd" 到达引擎时已是 "passwd"，裸控制字符则直接被 multipart
// 头解析拒绝。引擎能实际看到的是内嵌 ".." 与 URL 编码空字节等仍存活的特征。
func TestInspectUploadBadName(t *testing.T) {
	cases := []string{
		"a..b.txt",                   // 内嵌双点（穿越痕迹）
		"foo%00.php",                 // URL 编码空字节
		"..%2f..%2fetc%2fpasswd.txt", // URL 编码穿越串
	}
	for _, name := range cases {
		ct, body := buildMultipart(t, filePart{field: "file", filename: name, content: []byte("ok")})
		res := InspectUpload(ct, body)
		if !hasRule(res, UploadRuleBadName) {
			t.Errorf("文件名 %q 应命中非法文件名规则: %+v", name, res)
		}
	}
	// 裸穿越串被 Go 标准库预净化后（"passwd"）不该再命中，验证无误伤
	ct, body := buildMultipart(t, filePart{field: "file", filename: "../../etc/passwd", content: []byte("ok")})
	if res := InspectUpload(ct, body); res.Score != 0 {
		t.Fatalf("净化后的普通文件名不该命中: %+v", res)
	}
}

// TestInspectUploadTypeMismatch 验证声明图片但内容不符的两种检测。
func TestInspectUploadTypeMismatch(t *testing.T) {
	// 扩展名是图片但内容不是图片
	ct, body := buildMultipart(t, filePart{
		field: "file", filename: "photo.png", content: []byte("plain text, not an image"),
	})
	res := InspectUpload(ct, body)
	if !hasRule(res, UploadRuleMismatch) {
		t.Fatalf("伪图片应命中内容不符: %+v", res)
	}

	// 扩展名合法但声明的 Content-Type 是图片
	ct, body = buildMultipart(t, filePart{
		field: "file", filename: "avatar.bin", contentType: "image/png",
		content: []byte("definitely not png"),
	})
	res = InspectUpload(ct, body)
	if !hasRule(res, UploadRuleMismatch) {
		t.Fatalf("声明 image/png 应命中内容不符: %+v", res)
	}
}

// TestInspectUploadScriptMarkers 验证文件头中的脚本与可执行标记。
func TestInspectUploadScriptMarkers(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		score   int
	}{
		{name: "shebang", content: []byte("#!/bin/sh\nrm -rf /"), score: 70},
		{name: "php短标记", content: []byte("<?= system($_GET['c']) ?>"), score: 70},
		{name: "asp标记", content: []byte("<% Response.Write(1) %>"), score: 70},
		{name: "html脚本", content: []byte("<script>alert(1)</script>"), score: 70},
		{name: "ELF可执行", content: append([]byte{0x7F, 'E', 'L', 'F'}, bytes.Repeat([]byte{0}, 32)...), score: 80},
		{name: "PE可执行", content: append([]byte("MZ"), bytes.Repeat([]byte{0}, 32)...), score: 80},
		{name: "JavaClass", content: append([]byte{0xCA, 0xFE, 0xBA, 0xBE}, bytes.Repeat([]byte{0}, 32)...), score: 80},
	}
	for _, tc := range cases {
		ct, body := buildMultipart(t, filePart{
			field: "file", filename: "upload.bin", content: tc.content,
		})
		res := InspectUpload(ct, body)
		if !hasRule(res, UploadRuleScript) {
			t.Errorf("%s 应命中脚本/可执行规则: %+v", tc.name, res)
			continue
		}
		if res.Score < tc.score {
			t.Errorf("%s 评分应至少 %d: %d", tc.name, tc.score, res.Score)
		}
	}
}

// TestInspectUploadClean 验证正常上传不产生任何命中。
func TestInspectUploadClean(t *testing.T) {
	png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00}, 64)...)
	ct, body := buildMultipart(t,
		filePart{field: "avatar", filename: "avatar.png", content: png},
		filePart{field: "doc", filename: "readme.txt", content: []byte("hello world")},
	)
	res := InspectUpload(ct, body)
	if res.Score != 0 || len(res.Hits) != 0 {
		t.Fatalf("正常文件不该命中: %+v", res)
	}
}

// TestInspectUploadSkips 验证非 multipart、无文件名字段与空体的短路行为。
func TestInspectUploadSkips(t *testing.T) {
	// 空 body
	if res := InspectUpload("multipart/form-data; boundary=x", nil); res.Score != 0 {
		t.Fatal("空 body 应返回零值")
	}
	// 非 multipart 类型
	if res := InspectUpload("application/json", []byte(`{"a":1}`)); res.Score != 0 {
		t.Fatal("非 multipart 应返回零值")
	}
	// multipart 但没有 boundary
	if res := InspectUpload("multipart/form-data", []byte("data")); res.Score != 0 {
		t.Fatal("缺 boundary 应返回零值")
	}
	// 无 filename 的普通字段被跳过
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormField("note")
	fw.Write([]byte("just a field"))
	w.Close()
	if res := InspectUpload(w.FormDataContentType(), buf.Bytes()); res.Score != 0 {
		t.Fatalf("无文件名字段应被跳过: %+v", res)
	}
}

// TestInspectDeserializationJava 验证 Java 原生/编码序列化与利用链关键字。
func TestInspectDeserializationJava(t *testing.T) {
	// 二进制魔数 AC ED 00 05
	res := InspectDeserialization("application/octet-stream", []byte{0xAC, 0xED, 0x00, 0x05, 0x01, 0x02})
	if !hasRule(res, DeserJava) || res.Score < 85 {
		t.Fatalf("Java 魔数应命中 85 分: %+v", res)
	}
	// Base64 特征
	res = InspectDeserialization("application/x-www-form-urlencoded", []byte("payload=rO0ABQ=="))
	if !hasRule(res, DeserJava) || res.Score < 70 {
		t.Fatalf("rO0AB 特征应命中: %+v", res)
	}
	// JSON 上下文中的 ysoserial 利用链
	res = InspectDeserialization("application/json", []byte(`{"@type":"com.sun.rowset.JdbcRowSetImpl"}`))
	if !hasRule(res, DeserJava) {
		t.Fatalf("JdbcRowSetImpl 应命中利用链: %+v", res)
	}
}

// TestInspectDeserializationDotNet 验证 .NET BinaryFormatter 魔数。
func TestInspectDeserializationDotNet(t *testing.T) {
	magic := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}
	res := InspectDeserialization("application/octet-stream", magic)
	if !hasRule(res, DeserDotNet) || res.Score < 85 {
		t.Fatalf("BinaryFormatter 应命中: %+v", res)
	}
}

// TestInspectDeserializationPickle 验证 Python pickle 二进制与文本特征。
func TestInspectDeserializationPickle(t *testing.T) {
	// 协议头 0x80 0x02
	res := InspectDeserialization("application/octet-stream", []byte{0x80, 0x02, 0x01, 0x02})
	if !hasRule(res, DeserPickle) || res.Score < 85 {
		t.Fatalf("pickle 协议头应命中: %+v", res)
	}
	// 文本全局函数特征
	res = InspectDeserialization("application/octet-stream", []byte("cos\nsystem\nS'ls'"))
	if !hasRule(res, DeserPickle) || res.Score < 75 {
		t.Fatalf("pickle 全局函数应命中: %+v", res)
	}
	// 小体积 body 里的 Base64 载荷
	res = InspectDeserialization("application/json", []byte(`{"d":"gAJ9cQ=="}`))
	if !hasRule(res, DeserPickle) {
		t.Fatalf("Base64 pickle 特征应命中: %+v", res)
	}
}

// TestInspectDeserializationPHP 验证 PHP 序列化对象特征。
func TestInspectDeserializationPHP(t *testing.T) {
	res := InspectDeserialization("text/plain", []byte(`O:8:"stdClass":1:{s:3:"cmd";s:2:"id";}`))
	if !hasRule(res, DeserPHP) || res.Score < 60 {
		t.Fatalf("PHP 序列化应命中: %+v", res)
	}
}

// TestInspectDeserializationAutoType 验证 Fastjson/Jackson @type 检测。
func TestInspectDeserializationAutoType(t *testing.T) {
	res := InspectDeserialization("application/json", []byte(`{"@type":"org.apache.commons.configuration.JNDIConfiguration"}`))
	if !hasRule(res, DeserAutoType) || res.Score < 75 {
		t.Fatalf("@type 指向敏感类应命中: %+v", res)
	}
	// 普通 JSON 不该命中
	res = InspectDeserialization("application/json", []byte(`{"name":"tom","age":18}`))
	if res.Score != 0 {
		t.Fatalf("普通 JSON 不该命中: %+v", res)
	}
}

// TestInspectDeserializationYAML 验证 YAML 不安全标签。
func TestInspectDeserializationYAML(t *testing.T) {
	res := InspectDeserialization("application/yaml", []byte("!!python/object/apply:os.system ['ls']"))
	if !hasRule(res, DeserYAML) || res.Score < 70 {
		t.Fatalf("YAML 不安全标签应命中: %+v", res)
	}
}

// TestInspectDeserializationPrototype 验证原型链污染只在 JSON/表单上下文检测。
func TestInspectDeserializationPrototype(t *testing.T) {
	payload := []byte(`{"__proto__":{"isAdmin":true}}`)
	// JSON 上下文：命中
	res := InspectDeserialization("application/json", payload)
	if !hasRule(res, DeserPrototype) {
		t.Fatalf("JSON __proto__ 应命中: %+v", res)
	}
	// 纯文本上下文：不命中（降低误报）
	res = InspectDeserialization("text/plain", payload)
	if hasRule(res, DeserPrototype) {
		t.Fatalf("非 JSON/表单上下文不该命中: %+v", res)
	}
}

// TestInspectDeserializationClean 验证正常载荷不产生命中。
func TestInspectDeserializationClean(t *testing.T) {
	cases := []struct {
		ct   string
		body []byte
	}{
		{"application/json", []byte(`{"title":"Go 实战","price":59.9}`)},
		{"application/x-www-form-urlencoded", []byte("name=tom&age=18")},
		{"text/plain", []byte("hello world")},
	}
	for _, tc := range cases {
		if res := InspectDeserialization(tc.ct, tc.body); res.Score != 0 {
			t.Errorf("正常载荷 %q 不该命中: %+v", tc.body, res)
		}
	}
}

// TestInspectPayloadMerged 验证统一入口同时返回上传与反序列化结果。
func TestInspectPayloadMerged(t *testing.T) {
	// multipart 上传 PHP：只有 upload 类别
	ct, body := buildMultipart(t, filePart{
		field: "file", filename: "shell.php", content: []byte("<?php echo 1;"),
	})
	res := InspectPayload(ct, body)
	if !hasRule(res, UploadRuleExt) {
		t.Fatalf("统一入口应包含上传检测: %+v", res)
	}
	if len(res.Categories) != 1 || res.Categories[0] != "upload" {
		t.Fatalf("类别应只有 upload: %v", res.Categories)
	}

	// JSON @type 载荷：只有 deserialization 类别
	res = InspectPayload("application/json", []byte(`{"@type":"com.sun.rowset.JdbcRowSetImpl"}`))
	if !hasRule(res, DeserAutoType) || !hasRule(res, DeserJava) {
		t.Fatalf("统一入口应包含反序列化检测: %+v", res)
	}
	if len(res.Categories) != 1 || res.Categories[0] != "deserialization" {
		t.Fatalf("类别应只有 deserialization: %v", res.Categories)
	}

	// 空 body 短路
	if res := InspectPayload("application/json", nil); res.Score != 0 {
		t.Fatal("空 body 应返回零值")
	}
}

// TestTruncateEvidence 验证证据截断上限。
func TestTruncateEvidence(t *testing.T) {
	long := strings.Repeat("x", 300)
	if got := truncateEvidence(long); len(got) != 160 {
		t.Fatalf("证据应截断到 160: %d", len(got))
	}
	if got := truncateEvidence("short"); got != "short" {
		t.Fatalf("短证据不该被截断: %q", got)
	}
}
