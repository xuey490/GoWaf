package edge

import (
	"net/http/httptest"
	"testing"
)

// TestIsDocumentNavigation 验证「只挑战页面导航」的判定。
// 误判成 XHR 会让 SPA 的接口请求被塞回一张 HTML 挑战页，页面直接崩。
func TestIsDocumentNavigation(t *testing.T) {
	cases := []struct {
		name   string
		method string
		header map[string]string
		want   bool
	}{
		{"Chrome 顶层导航", "GET", map[string]string{"Sec-Fetch-Dest": "document"}, true},
		{"Chrome 图片子资源", "GET", map[string]string{"Sec-Fetch-Dest": "image"}, false},
		{"Chrome XHR", "GET", map[string]string{"Sec-Fetch-Dest": "empty"}, false},
		{"POST 表单提交", "POST", map[string]string{"Sec-Fetch-Dest": "document"}, false},
		{"老浏览器 Accept=html", "GET", map[string]string{"Accept": "text/html,application/xhtml+xml"}, true},
		{"老浏览器 Accept=json", "GET", map[string]string{"Accept": "application/json"}, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "http://demo.local/", nil)
		for k, v := range c.header {
			r.Header.Set(k, v)
		}
		if got := isDocumentNavigation(r); got != c.want {
			t.Errorf("%s: 期望 %v，实际 %v", c.name, c.want, got)
		}
	}
}

// TestHostMatchesPattern 验证站点域名与允许来路的匹配规则（含泛域名）。
func TestHostMatchesPattern(t *testing.T) {
	cases := []struct {
		host    string
		pattern string
		want    bool
	}{
		{"demo.local", "demo.local", true},
		{"demo.local", "other.local", false},
		{"a.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
		{"www.baidu.com", "www.baidu.com", true},
		{"", "www.baidu.com", false},
		{"demo.local", "", false},
	}
	for _, c := range cases {
		if got := hostMatchesPattern(c.host, c.pattern); got != c.want {
			t.Errorf("host=%q pattern=%q 期望 %v 实际 %v", c.host, c.pattern, c.want, got)
		}
	}
}

// TestHostInAllowList 验证逗号分隔的允许清单（含空格与泛域名）。
func TestHostInAllowList(t *testing.T) {
	list := "www.baidu.com, *.example.com ,www.google.com"
	if !hostInAllowList("www.baidu.com", list) {
		t.Fatal("精确匹配失败")
	}
	if !hostInAllowList("img.example.com", list) {
		t.Fatal("泛域名匹配失败")
	}
	if hostInAllowList("evil.com", list) {
		t.Fatal("未列入清单的域名不应命中")
	}
	if hostInAllowList("www.baidu.com.evil.com", list) {
		t.Fatal("后缀伪装域名不应命中")
	}
}
