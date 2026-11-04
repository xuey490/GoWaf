package notify

import (
	"io"
	"mime/quotedprintable"
	"strings"
	"testing"
)

// TestBuildMessageLineLength 回归：QQ 邮箱对超过 998 octet 的文本行直接回
// "500 Line too long, exceeds the 998-octet text line limit defined in RFC 5321"。
// 压缩后的 HTML 恰好就是超长单行，必须确保编出来的报文每行都够短。
func TestBuildMessageLineLength(t *testing.T) {
	// 模拟压缩后的 HTML：一行 6000 多字
	body := "<html><body><table>" + strings.Repeat("<tr><td>192.0.2.1</td><td>恶意扫描</td></tr>", 200) + "</table></body></html>"
	cfg := Config{Host: "smtp.qq.com", To: []string{"a@example.com", "b@example.com"}}

	raw, err := buildMessage(cfg, "waf@example.com", "【GoWAF 告警】封禁通知", body)
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}

	for i, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 {
			t.Fatalf("第 %d 行长度 %d 超过 RFC 5321 的 998 限制", i+1, len(line))
		}
		if len(line) > 78 && i > 6 { // 头部 7 行内允许稍宽（Date/Subject 等）
			t.Fatalf("第 %d 行长度 %d 过宽（正文应 ≤76）", i+1, len(line))
		}
	}

	if !strings.Contains(string(raw), "Content-Transfer-Encoding: quoted-printable") {
		t.Fatal("报文缺少 quoted-printable 头")
	}

	// 正文必须能原样解回来
	head, encoded, ok := strings.Cut(string(raw), "\r\n\r\n")
	if !ok {
		t.Fatal("报文缺少头部/正文分隔")
	}
	if !strings.Contains(head, "Subject: =?UTF-8?") {
		t.Fatalf("中文主题未做 RFC 2047 编码: %s", head)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(encoded)))
	if err != nil {
		t.Fatalf("quoted-printable 解码失败: %v", err)
	}
	if string(decoded) != body {
		t.Fatalf("正文解码不一致：got %d 字节, want %d 字节", len(decoded), len(body))
	}
}

// TestFoldAddressList 收件人很多时头部要折行，且不能把地址拆断。
func TestFoldAddressList(t *testing.T) {
	var to []string
	for i := 0; i < 10; i++ {
		to = append(to, "security-team-"+string(rune('a'+i))+"@example.com")
	}
	got := foldAddressList(to)
	for _, line := range strings.Split(got, "\r\n") {
		if len(line) > 78 {
			t.Fatalf("折行后仍有 %d 字符的行: %q", len(line), line)
		}
	}
	// 折行只发生在逗号之后，还原后应与原列表等价
	flat := strings.ReplaceAll(got, "\r\n ", " ")
	if strings.Join(strings.Split(flat, ", "), "|") != strings.Join(to, "|") {
		t.Fatalf("折行破坏了地址列表: %q", got)
	}
}
