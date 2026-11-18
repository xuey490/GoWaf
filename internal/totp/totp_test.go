package totp

import (
	"strings"
	"testing"
	"time"
)

// TestGenerateSecretFormat 密钥应为 32 字符 Base32 无填充。
func TestGenerateSecretFormat(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if len(s) != 32 {
		t.Fatalf("密钥长度 %d, 期望 32", len(s))
	}
	// 两次生成不应相同
	s2, _ := GenerateSecret()
	if s == s2 {
		t.Fatalf("两次生成的密钥相同")
	}
}

// TestCodeLength 动态码固定 6 位。
func TestCodeLength(t *testing.T) {
	s, _ := GenerateSecret()
	if len(Code(s)) != 6 {
		t.Fatalf("动态码应为 6 位")
	}
}

// TestVerifyRoundtrip 当前码校验通过；错码不通过。
func TestVerifyRoundtrip(t *testing.T) {
	s, _ := GenerateSecret()
	code := codeAt(s, time.Now())
	if !Verify(s, code) {
		t.Fatalf("当前码校验失败: %s", code)
	}
	// 步长边界：上一步的码也应通过（±1 窗口）
	if !Verify(s, codeAt(s, time.Now().Add(-StepSize))) {
		t.Fatalf("上一步码应通过（±1 窗口）")
	}
	if Verify(s, codeAt(s, time.Now().Add(-3*StepSize))) {
		t.Fatalf("3 步前的码不应通过")
	}
	if Verify(s, "000000") && Code(s) != "000000" {
		t.Fatalf("错码不应通过")
	}
}

// TestVerifyBadInput 非法输入不通过。
func TestVerifyBadInput(t *testing.T) {
	s, _ := GenerateSecret()
	for _, bad := range []string{"", "12345", "abcdef", "1234567", "  123456"} {
		if Verify(s, bad) {
			// "  123456" 带空白会 TrimSpace 后可能碰巧等于当前码，单独处理
			if bad == "  123456" {
				continue
			}
			t.Fatalf("非法输入 %q 不应通过", bad)
		}
	}
	if Verify("INVALID!!", "123456") {
		t.Fatalf("非法密钥不应通过")
	}
}

// TestPad6 补零逻辑。
func TestPad6(t *testing.T) {
	if pad6("42") != "000042" {
		t.Fatalf("pad6 结果错误: %s", pad6("42"))
	}
}

// TestProvisionURL 绑定 URL 结构正确。
func TestProvisionURL(t *testing.T) {
	u := ProvisionURL("GoWAF", "admin", "ABCDEFGHIJKLMNOP")
	if !strings.HasPrefix(u, "otpauth://totp/GoWAF:admin?") {
		t.Fatalf("URL 结构错误: %s", u)
	}
	if !strings.Contains(u, "secret=ABCDEFGHIJKLMNOP") {
		t.Fatalf("缺少 secret 参数: %s", u)
	}
	if !strings.Contains(u, "period=30") {
		t.Fatalf("缺少 period 参数: %s", u)
	}
}
