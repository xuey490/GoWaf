package edge

import "testing"

// 缓存匿名判定：WAF 自管 Cookie（人机验证通行证/策略认证通行证/控制台
// 会话）剥离后无剩余 Cookie 才算访客——业务会话 Cookie 照旧不缓存。
// 背景：guard.challenge.referer 默认开启，从搜索引擎跳转进来的访客通过
// 挑战后人手一张 waf_pass（30 分钟），若按 Cookie=="" 判匿名，guestOnly
// 缓存规则对绝大多数真实访客永不命中（v1.6.3 实测回归）。
func TestAnonymousGuest(t *testing.T) {
	cases := []struct {
		name   string
		cookie string
		auth   string
		want   bool
	}{
		{"无任何头", "", "", true},
		{"仅人机验证通行证", "waf_pass=abc123", "", true},
		{"仅策略认证通行证", "waf_auth_7=xyz", "", true},
		{"通行证加控制台会话", "waf_pass=abc; waf_token=t; waf_csrf=c", "", true},
		{"多段含业务会话", "waf_pass=abc; session=deadbeef", "", false},
		{"业务会话在前", "session=deadbeef; waf_pass=abc", "", false},
		{"大小写混合的通行证名", "WAF_PASS=abc", "", true},
		{"业务 Cookie 单独出现", "session=deadbeef", "", false},
		{"Authorization 头存在", "", "Bearer x", false},
		{"通行证加 Authorization", "waf_pass=abc", "Bearer x", false},
		{"空段与空白", " ; waf_pass=abc; ", "", true},
		{"同名前缀业务 Cookie", "waf_passwd=1", "", false},
	}
	for _, tc := range cases {
		if got := anonymousGuest(tc.cookie, tc.auth); got != tc.want {
			t.Errorf("%s: cookie=%q auth=%q got=%v want=%v", tc.name, tc.cookie, tc.auth, got, tc.want)
		}
	}
}

// waf_passwd 这类"以 waf_pass 开头但不是通行证"的名字必须按业务 Cookie 处理。
func TestIsWAFManagedCookieName(t *testing.T) {
	yes := []string{"waf_pass", "WAF_PASS", "waf_token", "waf_csrf", "waf_auth_12"}
	no := []string{"waf_passwd", "waf_pass2", "session", "waf", "waf_auth"}
	for _, n := range yes {
		if !isWAFManagedCookieName(n) {
			t.Errorf("%q 应识别为 WAF 自管 Cookie", n)
		}
	}
	for _, n := range no {
		if isWAFManagedCookieName(n) {
			t.Errorf("%q 不应识别为 WAF 自管 Cookie", n)
		}
	}
}
