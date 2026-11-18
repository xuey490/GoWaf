package acme

import (
	"net/http/httptest"
	"os"
	"testing"
)

// TestValidateDomain 域名校验表驱动测试。
func TestValidateDomain(t *testing.T) {
	cases := []struct {
		in     string
		wantOK bool
	}{
		{"example.com", true},
		{"*.example.com", true},
		{"a.b.example.com.", true}, // 尾点会被剥掉
		{"example.com/path", false},
		{"example.com:8080", false},
		{"localhost", false},
		{"", false},
	}
	for _, c := range cases {
		err := validateDomain(c.in)
		if (err == nil) != c.wantOK {
			t.Fatalf("validateDomain(%q) err=%v, 期望合法=%v", c.in, err, c.wantOK)
		}
	}
}

// TestServeChallenge 验证 HTTP-01 应答表命中与未命中两种路径。
func TestServeChallenge(t *testing.T) {
	m := New(Options{StateDir: t.TempDir()})
	m.mu.Lock()
	m.httpTokens["token-abc"] = &httpTokenEntry{keyAuth: "token-abc.thumbprint", orderID: "wo1"}
	m.mu.Unlock()

	// 命中：返回 200 + keyAuth 正文
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/.well-known/acme-challenge/token-abc", nil)
	if !m.ServeChallenge(w, r) {
		t.Fatalf("已登记令牌应被应答")
	}
	if w.Body.String() != "token-abc.thumbprint" {
		t.Fatalf("应答正文不符: %s", w.Body.String())
	}
	if w.Header().Get("Cache-Control") == "" {
		t.Fatalf("挑战应答必须带 no-store")
	}

	// 未命中：交还调用方
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/.well-known/acme-challenge/unknown-token", nil)
	if m.ServeChallenge(w2, r2) {
		t.Fatalf("未知令牌不应被应答")
	}

	// 非挑战路径：直接交还
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest("GET", "/normal/page", nil)
	if m.ServeChallenge(w3, r3) {
		t.Fatalf("非挑战路径不应被处理")
	}
}

// TestCancelOrder 验证取消订单同时清理 HTTP-01 应答表。
func TestCancelOrder(t *testing.T) {
	m := New(Options{StateDir: t.TempDir()})
	m.mu.Lock()
	m.orders["wo9"] = &Order{ID: "wo9"}
	m.httpTokens["tok9"] = &httpTokenEntry{keyAuth: "k", orderID: "wo9"}
	m.mu.Unlock()

	if !m.CancelOrder("wo9") {
		t.Fatalf("存在的订单应可取消")
	}
	if m.CancelOrder("wo9") {
		t.Fatalf("重复取消应返回 false")
	}
	m.mu.Lock()
	_, ok := m.httpTokens["tok9"]
	m.mu.Unlock()
	if ok {
		t.Fatalf("取消订单后应清理挑战应答表")
	}
}

// TestAccountForCreatesKey 验证账户密钥生成与持久化（不触网：Register 需要网络，
// 因此这里只验证本地文件行为——StateDir 下应生成 account-*.key）。
func TestAccountForPersistence(t *testing.T) {
	// accountFor 会尝试注册（触网失败），此处仅测试密钥文件先于注册写出。
	dir := t.TempDir()
	m := New(Options{StateDir: dir})
	_, _ = m.accountFor("https://example.invalid/directory", "")
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatalf("未生成账户密钥文件")
	}
}
