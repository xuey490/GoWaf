package detect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// resetCRSUpdateCache 清空检查结果缓存，保证每个用例都真正出网。
func resetCRSUpdateCache() {
	crsUpdateMu.Lock()
	defer crsUpdateMu.Unlock()
	crsUpdateCache = nil
}

// TestCRSVersionLess 验证版本比较：逐段数值比较、前导 v/缺省段/后缀容忍、
// 以及解析失败时退化为字符串比较的行为。
func TestCRSVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v4.30.0", "v4.31.0", true},      // 上游 patch+1 → 有更新
		{"v4.30.0", "v5.0.0", true},       // 上游大版本更新
		{"v4.30.0", "v4.30.0", false},     // 同版本
		{"v4.31.0", "v4.30.0", false},     // 当前更新（不该发生，但别误报）
		{"v4.30.0", "4.30.1", true},       // 上游缺前导 v 也能比
		{"v4", "v4.0.1", true},            // 缺省段按 0 补齐
		{"v4.30.0", "v4.30.0-rc1", false}, // 后缀在比较前被剥掉，视为同版本
		{"v4.30.0", "latest", true},       // 上游返回不认识的串 → 退化为字符串不等 → 报有更新
		{"v9.9.9", "latest", true},
	}
	for _, tc := range cases {
		if got := crsVersionLess(tc.a, tc.b); got != tc.want {
			t.Errorf("crsVersionLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestParseCRSVer 验证版本号解析：正常三段、前导 v、后缀剥离与非法输入。
func TestParseCRSVer(t *testing.T) {
	if p, ok := parseCRSVer(" v4.30.0 "); !ok || p != [3]int{4, 30, 0} {
		t.Errorf("parseCRSVer(v4.30.0) = %v, %v; want [4 30 0], true", p, ok)
	}
	if p, ok := parseCRSVer("4.30"); !ok || p != [3]int{4, 30, 0} {
		t.Errorf("parseCRSVer(4.30) = %v, %v; want [4 30 0], true", p, ok)
	}
	// 预发布后缀（含多段）在比较前被整体剥离，取基础版本
	if p, ok := parseCRSVer("v4.30.0-rc1.2.3.4"); !ok || p != [3]int{4, 30, 0} {
		t.Errorf("parseCRSVer(v4.30.0-rc1.2.3.4) = %v, %v; want [4 30 0], true", p, ok)
	}
	for _, bad := range []string{"", "v", "v4.x.0", "abc"} {
		if _, ok := parseCRSVer(bad); ok {
			t.Errorf("parseCRSVer(%q) 解析成功, 期望失败", bad)
		}
	}
}

// TestCheckCRSUpdateGitHub 验证主数据源：GitHub release 接口正常时
// 直接采用其版本号与 release 页链接，并正确判定 updateAvailable。
func TestCheckCRSUpdateGitHub(t *testing.T) {
	resetCRSUpdateCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v4.31.0","html_url":"https://github.com/coreruleset/coreruleset/releases/tag/v4.31.0","published_at":"2026-11-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	oldAPI := crsReleaseAPI
	crsReleaseAPI = srv.URL + "/repos/coreruleset/coreruleset/releases/latest"
	defer func() { crsReleaseAPI = oldAPI }()

	info, err := CheckCRSUpdate(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckCRSUpdate 失败: %v", err)
	}
	if info.Latest != "v4.31.0" || !info.UpdateAvailable {
		t.Errorf("Latest=%q UpdateAvailable=%v; want v4.31.0, true", info.Latest, info.UpdateAvailable)
	}
	if info.ReleaseURL != "https://github.com/coreruleset/coreruleset/releases/tag/v4.31.0" {
		t.Errorf("ReleaseURL = %q, 未采用 GitHub 返回的单版链接", info.ReleaseURL)
	}
	if info.Cached {
		t.Error("force=true 的首次检查不应命中缓存")
	}

	// 第二次非强制调用应命中缓存且保留原 CheckedAt
	again, err := CheckCRSUpdate(context.Background(), false)
	if err != nil {
		t.Fatalf("缓存读取失败: %v", err)
	}
	if !again.Cached || again.Latest != "v4.31.0" {
		t.Errorf("缓存命中异常: Cached=%v Latest=%q", again.Cached, again.Latest)
	}
}

// TestCheckCRSUpdateFallback 验证回退链：GitHub 挂掉后依次尝试模块代理，
// 任一成功即采用其版本号，release 链接退化为列表页。
func TestCheckCRSUpdateFallback(t *testing.T) {
	resetCRSUpdateCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/github":
			http.Error(w, "rate limited", http.StatusForbidden) // 模拟 GitHub 限流
		case r.URL.Path == "/proxy2":
			_, _ = w.Write([]byte(`{"Version":"v4.32.0","Time":"2026-11-02T00:00:00Z"}`))
		default:
			http.Error(w, "down", http.StatusInternalServerError) // /proxy1 不可用
		}
	}))
	defer srv.Close()

	oldAPI, oldProxies := crsReleaseAPI, crsProxyAPIs
	crsReleaseAPI = srv.URL + "/github"
	crsProxyAPIs = []string{srv.URL + "/proxy1", srv.URL + "/proxy2"}
	defer func() { crsReleaseAPI, crsProxyAPIs = oldAPI, oldProxies }()

	info, err := CheckCRSUpdate(context.Background(), true)
	if err != nil {
		t.Fatalf("回退链检查失败: %v", err)
	}
	if info.Latest != "v4.32.0" || !info.UpdateAvailable {
		t.Errorf("Latest=%q UpdateAvailable=%v; want v4.32.0, true", info.Latest, info.UpdateAvailable)
	}
	if info.ReleaseURL != crsReleasesPage {
		t.Errorf("ReleaseURL = %q, 兜底源应退化为列表页 %q", info.ReleaseURL, crsReleasesPage)
	}
}

// TestCheckCRSUpdateAllDown 验证全部数据源不可达时返回错误而非半成品结果。
func TestCheckCRSUpdateAllDown(t *testing.T) {
	resetCRSUpdateCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	oldAPI, oldProxies := crsReleaseAPI, crsProxyAPIs
	crsReleaseAPI = srv.URL + "/github"
	crsProxyAPIs = []string{srv.URL + "/proxy1", srv.URL + "/proxy2"}
	defer func() { crsReleaseAPI, crsProxyAPIs = oldAPI, oldProxies }()

	if _, err := CheckCRSUpdate(context.Background(), true); err == nil {
		t.Fatal("全部数据源不可达应返回错误")
	}
}
