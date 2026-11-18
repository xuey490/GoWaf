package guard

import (
	"strings"
	"testing"
	"time"
)

// TestCaptchaRoundtrip 签发→正确答案应通过；错答案不通过。
func TestCaptchaRoundtrip(t *testing.T) {
	c := NewChallenger([]byte("test-secret-captcha"), 30*time.Minute)
	ip := "1.2.3.4"
	now := time.Now()
	token, question := c.CaptchaToken(ip, now)
	if question == "" || !strings.ContainsAny(question, "+-") {
		t.Fatalf("算式非法: %s", question)
	}

	// 从算式还原答案（测试直接计算）
	ans, err := evalExpr(question)
	if err != nil {
		t.Fatalf("算式解析失败: %v", err)
	}
	if !c.VerifyCaptcha(token, ip, ans, time.Now()) {
		t.Fatalf("正确答案应通过")
	}
	if c.VerifyCaptcha(token, ip, "99", time.Now()) {
		t.Fatalf("错误答案不应通过")
	}
}

// TestCaptchaWrongIP 令牌绑定 IP，换 IP 校验失败。
func TestCaptchaWrongIP(t *testing.T) {
	c := NewChallenger([]byte("test-secret-captcha"), 30*time.Minute)
	token, question := c.CaptchaToken("1.2.3.4", time.Now())
	ans, _ := evalExpr(question)
	if c.VerifyCaptcha(token, "5.6.7.8", ans, time.Now()) {
		t.Fatalf("换 IP 校验不应通过")
	}
}

// TestCaptchaExpired 过期令牌不通过。
func TestCaptchaExpired(t *testing.T) {
	c := NewChallenger([]byte("test-secret-captcha"), 30*time.Minute)
	token, question := c.CaptchaToken("1.2.3.4", time.Now().Add(-2*captchaTTL))
	ans, _ := evalExpr(question)
	if c.VerifyCaptcha(token, "1.2.3.4", ans, time.Now()) {
		t.Fatalf("过期令牌不应通过")
	}
}

// TestCaptchaTampered 篡改令牌不通过。
func TestCaptchaTampered(t *testing.T) {
	c := NewChallenger([]byte("test-secret-captcha"), 30*time.Minute)
	token, question := c.CaptchaToken("1.2.3.4", time.Now())
	ans, _ := evalExpr(question)
	bad := token[:len(token)-4] + "AAAA"
	if c.VerifyCaptcha(bad, "1.2.3.4", ans, time.Now()) {
		t.Fatalf("篡改令牌不应通过")
	}
}

// TestCaptchaSVG SVG 应包含算式字符。
func TestCaptchaSVG(t *testing.T) {
	svg := string(CaptchaSVG("7+5"))
	if !strings.Contains(svg, "7") || !strings.Contains(svg, "5") {
		t.Fatalf("SVG 未包含算式字符")
	}
}

// evalExpr 解析 "a+b"/"a-b" 算式（测试专用）。
func evalExpr(expr string) (string, error) {
	sign := "+"
	if strings.Contains(expr, "-") {
		sign = "-"
	}
	idx := strings.IndexByte(expr, sign[0])
	if idx <= 0 {
		return "", &strconvNumError{expr: expr}
	}
	a, b := expr[:idx], expr[idx+1:]
	return parseEval(a, b, sign), nil
}

// strconvNumError 测试专用错误。
type strconvNumError struct{ expr string }

func (e *strconvNumError) Error() string { return "bad expr: " + e.expr }

func parseEval(a, b, sign string) string {
	ai, bi := 0, 0
	for _, c := range a {
		ai = ai*10 + int(c-'0')
	}
	for _, c := range b {
		bi = bi*10 + int(c-'0')
	}
	if sign == "-" {
		ai -= bi
	} else {
		ai += bi
	}
	return strings.TrimSpace(itoa(ai))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
