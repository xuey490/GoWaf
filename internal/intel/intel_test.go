package intel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"gowaf/internal/store/model"
)

// TestSyncClearsWhenAllSourcesRemoved 回归：删掉/停用最后一个情报源后，
// 内存快照必须清空，否则已删除的情报会一直拦下去。
func TestSyncClearsWhenAllSourcesRemoved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# feed\n203.0.113.0/24\n198.51.100.7\n"))
	}))
	defer srv.Close()

	m := New()
	src := model.IntelSource{ID: 1, Name: "test", URL: srv.URL, Scope: "global", Enabled: 1}
	m.Sync(context.Background(), []model.IntelSource{src})

	bad := netip.MustParseAddr("203.0.113.9")
	if got := m.Match(bad, ""); got == "" {
		t.Fatal("情报未生效，用例前提不成立")
	}
	if m.GlobalCount() == 0 {
		t.Fatal("全局区间数为 0")
	}

	// 源被删除 / 停用：传入空列表
	m.Sync(context.Background(), nil)
	if got := m.Match(bad, ""); got != "" {
		t.Fatalf("源已删除但仍在拦截: %s", got)
	}
	if n := m.GlobalCount(); n != 0 {
		t.Fatalf("全局区间未清空: %d", n)
	}
	if len(m.Stats()) != 0 {
		t.Fatalf("状态未清理: %+v", m.Stats())
	}
}

// TestSiteScopeIsolation 验证站点级情报不影响其他站点。
func TestSiteScopeIsolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("192.0.2.10-192.0.2.20\n"))
	}))
	defer srv.Close()

	m := New()
	m.Sync(context.Background(), []model.IntelSource{
		{ID: 1, Name: "site-feed", URL: srv.URL, Scope: "site", Site: "shop", Enabled: 1},
	})

	ip := netip.MustParseAddr("192.0.2.15")
	if got := m.Match(ip, "shop"); got != "站点情报" {
		t.Fatalf("站点情报未命中: %q", got)
	}
	if got := m.Match(ip, "other"); got != "" {
		t.Fatalf("站点情报泄漏到其他站点: %q", got)
	}
	if n := m.GlobalCount(); n != 0 {
		t.Fatalf("站点级源不应计入全局: %d", n)
	}
}
