package edge

import (
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gowaf/internal/policy"
)

// testAuthSecret 测试用的 policy-auth 密钥。
var testAuthSecret = []byte("test-policy-auth-secret-0123456789")

// TestSignVerifyPolicyAuthToken 验证策略认证通行证的签发与校验闭环。
func TestSignVerifyPolicyAuthToken(t *testing.T) {
	now := time.Now()
	token := signPolicyAuthToken(testAuthSecret, 7, "1.2.3.4", now.Add(time.Hour).UnixMilli())
	if !verifyPolicyAuthTokenWith(testAuthSecret, token, 7, "1.2.3.4", now) {
		t.Fatalf("合法通行证应通过校验")
	}
	// IP 变化 → 拒绝（通行证绑定 IP）
	if verifyPolicyAuthTokenWith(testAuthSecret, token, 7, "5.6.7.8", now) {
		t.Fatalf("IP 不一致应拒绝")
	}
	// 策略 ID 变化 → 拒绝（Cookie 按策略独立）
	if verifyPolicyAuthTokenWith(testAuthSecret, token, 8, "1.2.3.4", now) {
		t.Fatalf("策略 ID 不一致应拒绝")
	}
	// 过期 → 拒绝
	expired := signPolicyAuthToken(testAuthSecret, 7, "1.2.3.4", now.Add(-time.Minute).UnixMilli())
	if verifyPolicyAuthTokenWith(testAuthSecret, expired, 7, "1.2.3.4", now) {
		t.Fatalf("过期通行证应拒绝")
	}
	// 密钥不同 → 拒绝（跨环境伪造）
	if verifyPolicyAuthTokenWith([]byte("other-secret"), token, 7, "1.2.3.4", now) {
		t.Fatalf("密钥不一致应拒绝")
	}
	// 篡改与垃圾输入 → 拒绝
	if verifyPolicyAuthTokenWith(testAuthSecret, token+"x", 7, "1.2.3.4", now) {
		t.Fatalf("篡改的通行证应拒绝")
	}
	if verifyPolicyAuthTokenWith(testAuthSecret, "garbage", 7, "1.2.3.4", now) {
		t.Fatalf("垃圾输入应拒绝")
	}
}

// TestSafeRedirectTarget 验证开放重定向防护。
func TestSafeRedirectTarget(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/dashboard", "/dashboard"},
		{"", "/"},
		{"//evil.com", "/"},
		{"https://evil.com", "/"},
		{"/a?b=1&c=2", "/a?b=1&c=2"},
	}
	for _, c := range cases {
		if got := safeRedirectTarget(c.in); got != c.want {
			t.Fatalf("safeRedirectTarget(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

// authTestHash 在测试初始化时生成明文 "secret123" 的 bcrypt 哈希
// （bcrypt 生成约几十毫秒，TestMain 一次性完成，避免固定向量与明文不符）。
var authTestHash string

func init() {
	b, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	authTestHash = string(b)
}

// TestBasicAuthValidShape 验证 basic 认证凭据的形状判定。
// 注意夹具在测试函数内构建：包级变量初始化先于 init() 执行，
// 那时 authTestHash 还是空串，会导致比对永远失败。
func TestBasicAuthValidShape(t *testing.T) {
	cfg := &policy.AuthConfig{
		Mode:  "basic",
		Users: []policy.AuthUser{{User: "admin", PasswordHash: authTestHash}},
	}
	// 无 Authorization 头
	r := httptest.NewRequest("GET", "/x", nil)
	if basicAuthValid(cfg, r) {
		t.Fatalf("无凭据不应通过")
	}
	// 非 Basic 凭据
	r2 := httptest.NewRequest("GET", "/x", nil)
	r2.Header.Set("Authorization", "Bearer whatever")
	if basicAuthValid(cfg, r2) {
		t.Fatalf("非 Basic 凭据不应通过")
	}
	// 正确格式 + 正确密码
	r3 := httptest.NewRequest("GET", "/x", nil)
	r3.SetBasicAuth("admin", "secret123")
	if !basicAuthValid(cfg, r3) {
		t.Fatalf("正确凭据应通过")
	}
	// 正确格式 + 错误密码
	r4 := httptest.NewRequest("GET", "/x", nil)
	r4.SetBasicAuth("admin", "wrong")
	if basicAuthValid(cfg, r4) {
		t.Fatalf("错误密码不应通过")
	}
}
