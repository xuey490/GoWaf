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
	// 本地测试源：返回一份带注释行的纯文本清单
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# feed\n203.0.113.0/24\n198.51.100.7\n"))
	}))
	defer srv.Close()

	m := New()
	// 一个启用中的全局源
	src := model.IntelSource{ID: 1, Name: "test", URL: srv.URL, Scope: "global", Enabled: 1}
	m.Sync(context.Background(), []model.IntelSource{src})

	// TEST-NET-3 段内的地址：未被特殊用途过滤排除，作为命中样本
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
	// 状态表里的幽灵条目也应一并清理
	if len(m.Stats()) != 0 {
		t.Fatalf("状态未清理: %+v", m.Stats())
	}
}

// TestSiteScopeIsolation 验证站点级情报的隔离性：只对绑定的站点生效，
// 不泄漏到其他站点，也不计入全局区间数。
func TestSiteScopeIsolation(t *testing.T) {
	// 测试源返回一段 IPv4 起止区间（TEST-NET-1 段）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("192.0.2.10-192.0.2.20\n"))
	}))
	defer srv.Close()

	m := New()
	// 站点级源：只对 shop 生效
	m.Sync(context.Background(), []model.IntelSource{
		{ID: 1, Name: "site-feed", URL: srv.URL, Scope: "site", Site: "shop", Enabled: 1},
	})

	// 区间内的地址
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
