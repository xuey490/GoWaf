package dprotect

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

// spanPattern 从加密后的页面里提取占位节点（class 与密文）。
var spanPattern = regexp.MustCompile(`<span class="wafx">([A-Za-z0-9+/=]+)</span>`)

// keyPattern 从注入脚本里提取混淆密钥数组。
var keyPattern = regexp.MustCompile(`window\.__wafxK=\[([0-9,]+)\]`)

// TestProtectRoundtrip 加密→按页面内嵌密钥解密，应还原全部明文文本。
// 该测试同时校验「混淆密钥 + RC4-drop256 + base64」整条链路的自洽性；
// 浏览器端 JS 解密器按同一算法实现（decrypt.js），由维护者在真实浏览器里验收。
func TestProtectRoundtrip(t *testing.T) {
	m, err := New([]byte("test-secret-0123456789abcdef"))
	if err != nil {
		t.Fatalf("创建管理器失败: %v", err)
	}
	original := `<html><body><h1>你好，世界</h1><p>访问计数 1024 次</p></body></html>`
	out, err := m.Protect([]byte(original))
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	page := string(out)

	// 密文不应包含原文
	if strings.Contains(page, "你好，世界") || strings.Contains(page, "访问计数") {
		t.Fatalf("明文未被加密")
	}
	// 解密脚本已注入：混淆密钥参数与查询 span.wafx 的脚本必须同时存在
	if !strings.Contains(page, "__wafxK") || !strings.Contains(page, "span.wafx") {
		t.Fatalf("解密脚本未注入")
	}

	// 还原密钥并解密每个占位节点
	keyMatch := keyPattern.FindStringSubmatch(page)
	if keyMatch == nil {
		t.Fatalf("未找到混淆密钥")
	}
	var obf []byte
	for _, part := range strings.Split(keyMatch[1], ",") {
		b := 0
		for _, r := range part {
			b = b*10 + int(r-'0')
		}
		obf = append(obf, byte(b))
	}
	key := make([]byte, len(obf))
	for i, b := range obf {
		key[i] = b ^ 0x5A ^ byte(i&7)
	}

	var restored strings.Builder
	restored.WriteString("Recovered:") // 锚点，防止后续 Contains 匹配到源码里的示例文本
	matches := spanPattern.FindAllStringSubmatch(page, -1)
	if len(matches) != 2 {
		t.Fatalf("占位节点数量 = %d, 期望 2", len(matches))
	}
	for _, mk := range matches {
		cipher, err := base64.StdEncoding.DecodeString(mk[1])
		if err != nil {
			t.Fatalf("密文 base64 解码失败: %v", err)
		}
		restored.Write(cryptXOR(cipher, key))
	}
	if !strings.Contains(restored.String(), "你好，世界") || !strings.Contains(restored.String(), "访问计数 1024 次") {
		t.Fatalf("解密结果不符合原文: %s", restored.String())
	}
}

// TestProtectSkipsScripts 验证 script/style/textarea 内容原样保留。
func TestProtectSkipsScripts(t *testing.T) {
	m, _ := New([]byte("test-secret-0123456789abcdef"))
	original := `<html><head><script>var total = 1024;</script><style>.a{color:red}</style></head>` +
		`<body><textarea>预填值文本</textarea><p>正文内容</p></body></html>`
	out, err := m.Protect([]byte(original))
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	page := string(out)
	if !strings.Contains(page, "var total = 1024;") {
		t.Fatalf("script 内容被改动")
	}
	if !strings.Contains(page, ".a{color:red}") {
		t.Fatalf("style 内容被改动")
	}
	if !strings.Contains(page, "预填值文本") {
		t.Fatalf("textarea 内容被改动")
	}
}

// TestProtectNoPlainTextNodes 纯空白/无文本页面不注入脚本。
func TestProtectNoPlainTextNodes(t *testing.T) {
	m, _ := New([]byte("test-secret-0123456789abcdef"))
	out, err := m.Protect([]byte(`<html><body><img src="x.png"></body></html>`))
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if strings.Contains(string(out), "__wafxK") {
		t.Fatalf("无可加密文本时不应注入脚本")
	}
}

// TestCryptInvolution 验证 RC4 密钥流异或是自反的。
func TestCryptInvolution(t *testing.T) {
	key := []byte("some-key-for-test")
	text := []byte("任何字节序列 0x00 0xff 混合")
	once := cryptXOR(text, key)
	twice := cryptXOR(once, key)
	if string(twice) != string(text) {
		t.Fatalf("异或加解密不自反")
	}
}

// TestNewRejectsShortSecret 验证根密钥长度校验。
func TestNewRejectsShortSecret(t *testing.T) {
	if _, err := New([]byte("short")); err == nil {
		t.Fatalf("过短密钥应被拒绝")
	}
}
