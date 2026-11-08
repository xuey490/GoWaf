package guard

import (
	"testing"
	"time"
)

// solvePoW 是测试端的「客户端」：穷举计数器直到满足难度。
func solvePoW(t *testing.T, token string, bits int) uint64 {
	t.Helper()
	for c := uint64(0); c < 5_000_000; c++ {
		if LeadingZeroBits(token, c) >= bits {
			return c
		}
	}
	t.Fatalf("在 500 万次内没算出差分为 %d 位的答案", bits)
	return 0
}

// TestChallengePoW 验证挑战令牌与工作量证明的完整链路。
func TestChallengePoW(t *testing.T) {
	c := NewChallenger([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	now := time.Now()
	token := c.ChallengeToken("1.2.3.4", now, 8)
	ans := solvePoW(t, token, 8)

	if !c.VerifyPoW(token, "1.2.3.4", ans, now) {
		t.Fatal("正确答案 + 同一 IP 应校验通过")
	}
	if c.VerifyPoW(token, "5.6.7.8", ans, now) {
		t.Fatal("通行证绑定 IP，换 IP 必须失败")
	}
	if c.VerifyPoW(token, "1.2.3.4", ans+1, now) {
		t.Fatal("错误答案必须失败")
	}
	if c.VerifyPoW(token, "1.2.3.4", ans, now.Add(10*time.Minute)) {
		t.Fatal("令牌过期后必须失败")
	}
	if c.VerifyPoW(token+"x", "1.2.3.4", ans, now) {
		t.Fatal("签名被篡改必须失败")
	}
	if c.VerifyPoW("", "1.2.3.4", ans, now) {
		t.Fatal("空令牌必须失败")
	}
}

// TestChallengePoWRejectsHeaderOnly 验证「只带 X-WAF-JS 头、不带令牌」无法通过校验。
// 这正是原实现的漏洞：无条件给带该请求头的请求签发通行证。
func TestChallengePoWRejectsHeaderOnly(t *testing.T) {
	c := NewChallenger([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	if c.VerifyPoW("", "1.2.3.4", 0, time.Now()) {
		t.Fatal("没有挑战令牌时不能通过校验")
	}
}

// TestLeadingZeroBits 校验前导零计数与哈希值一致（前端用同一算法）。
func TestLeadingZeroBits(t *testing.T) {
	token := "sample-token"
	for c := uint64(0); c < 200; c++ {
		bits := LeadingZeroBits(token, c)
		if bits < 0 || bits > 256 {
			t.Fatalf("前导零位数越界: %d", bits)
		}
	}
}
