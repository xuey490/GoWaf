package policy

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// bcryptHash 是一个真实有效的 bcrypt 哈希（明文 "secret123"，测试固定向量）。
const bcryptHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// TestCompileAuthConfig 验证 auth 动作配置的编译校验。
func TestCompileAuthConfig(t *testing.T) {
	conds := Group{Logic: "and", Items: []Condition{{Field: "path", Op: "prefix", Value: "/admin"}}}
	cases := []struct {
		name    string
		action  Action
		config  string
		wantErr bool
	}{
		{"basic 合法", ActionAuth, `{"mode":"basic","users":[{"user":"admin","passwordHash":"` + bcryptHash + `"}]}`, false},
		{"page 合法", ActionAuth, `{"mode":"page","passwordHash":"` + bcryptHash + `"}`, false},
		{"缺 config", ActionAuth, "", true},
		{"mode 非法", ActionAuth, `{"mode":"otp"}`, true},
		{"basic 无账号", ActionAuth, `{"mode":"basic"}`, true},
		{"哈希非 bcrypt", ActionAuth, `{"mode":"page","passwordHash":"plain-text"}`, true},
		{"rate_limit 合法", ActionRateLimit, `{"windowSec":60,"threshold":100,"onExceed":"challenge"}`, false},
		{"rate_limit 缺 config", ActionRateLimit, "", true},
		{"rate_limit 窗口过小", ActionRateLimit, `{"windowSec":5,"threshold":10}`, true},
		{"rate_limit 窗口过大", ActionRateLimit, `{"windowSec":90000,"threshold":10}`, true},
		{"rate_limit 动作非法", ActionRateLimit, `{"windowSec":60,"threshold":10,"onExceed":"ban"}`, true},
	}
	for _, c := range cases {
		set, err := Compile([]Definition{{
			ID: 1, Name: "t", Enabled: true, Action: c.action,
			Conditions: conds, Config: c.config,
		}})
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err=%v, 期望错误=%v", c.name, err, c.wantErr)
		}
		if !c.wantErr && set == nil {
			t.Fatalf("%s: 合法配置不应返回 nil", c.name)
		}
	}
}

// TestGateActionNeedsBody 验证门禁动作拒绝依赖 body 的条件。
func TestGateActionNeedsBody(t *testing.T) {
	conds := Group{Logic: "and", Items: []Condition{{Field: "body", Op: "contains", Value: "x"}}}
	_, err := Compile([]Definition{{
		ID: 1, Name: "gate", Enabled: true, Action: ActionAuth,
		Conditions: conds, Config: `{"mode":"page","passwordHash":"` + bcryptHash + `"}`,
	}})
	if err == nil || !strings.Contains(err.Error(), "不能依赖 body") {
		t.Fatalf("auth + body 条件应被拒绝, err=%v", err)
	}
}

// TestEvaluateSkipsGateActions 验证完整求值跳过门禁动作（防止二次触发）。
func TestEvaluateSkipsGateActions(t *testing.T) {
	conds := Group{Logic: "and", Items: []Condition{{Field: "path", Op: "prefix", Value: "/admin"}}}
	set, err := Compile([]Definition{
		{ID: 1, Name: "gate", Enabled: true, Priority: 1, Action: ActionAuth,
			Conditions: conds, Config: `{"mode":"page","passwordHash":"` + bcryptHash + `"}`},
		{ID: 2, Name: "block", Enabled: true, Priority: 2, Action: ActionBlock,
			Conditions: conds},
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	req := NewRequest(httptest.NewRequest("GET", "/admin/login", nil), nil, "1.2.3.4", "CN", "")

	// 早期求值：第一条命中是 auth 门禁 → 返回 auth 且带配置
	m, ok := set.EvaluateDenyEarly(req, "site1")
	if !ok || m.Action != ActionAuth {
		t.Fatalf("早期求值应命中 auth 门禁, got %v ok=%v", m, ok)
	}
	if m.Auth == nil || m.Auth.Mode != "page" {
		t.Fatalf("auth 参数应随命中带出: %+v", m.Auth)
	}

	// 完整求值：auth 被跳过 → 命中的是 block 策略
	m2, ok2 := set.Evaluate(req, "site1")
	if !ok2 || m2.Action != ActionBlock {
		t.Fatalf("完整求值应跳过 auth 命中 block, got %v ok=%v", m2, ok2)
	}
}

// TestAuthConfigByID 验证按 ID 直查认证配置（密码提交端点用）。
func TestAuthConfigByID(t *testing.T) {
	conds := Group{Logic: "and", Items: []Condition{{Field: "path", Op: "prefix", Value: "/vip"}}}
	set, err := Compile([]Definition{{
		ID: 7, Name: "vip", Enabled: true, Action: ActionAuth,
		Conditions: conds, Config: `{"mode":"page","passwordHash":"` + bcryptHash + `","ttlSec":3600}`,
	}})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	cfg, ok := set.AuthConfigByID(7)
	if !ok || cfg == nil || cfg.Mode != "page" || cfg.TTLSec != 3600 {
		t.Fatalf("AuthConfigByID(7) = %+v ok=%v", cfg, ok)
	}
	if _, ok := set.AuthConfigByID(99); ok {
		t.Fatalf("不存在的 ID 应返回 false")
	}
}

// TestPolicyAuthCookieName 验证 Cookie 命名约定。
func TestPolicyAuthCookieName(t *testing.T) {
	if got := PolicyAuthCookieName(42); got != "waf_auth_42" {
		t.Fatalf("cookie name = %s", got)
	}
}
