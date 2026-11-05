package runtime

import (
	"strings"
	"testing"

	"gowaf/internal/config"
	"gowaf/internal/guard"
	"gowaf/internal/proxy"
	"gowaf/internal/store/model"
)

// buildRT 构造只带路由快照与 trusted_proxies 的最小 Runtime，够跑配置自检。
// 参数 sites：进入路由表的站点；参数 trusted：server.trusted_proxies 的配置值。
// 返回 *Runtime：可直接调用 CDNWarnings 的实例。
func buildRT(t *testing.T, sites []model.Site, trusted []string) *Runtime {
	t.Helper()
	router, err := proxy.BuildRouter(sites, nil)
	if err != nil {
		t.Fatalf("构建路由失败: %v", err)
	}
	rt := &Runtime{Cfg: &config.Config{}}
	rt.Cfg.Server.TrustedProxies = trusted
	rt.router.Store(router)
	return rt
}

// cfTrustedAll 返回全部 Cloudflare 网段的 CIDR 字符串，模拟 trusted_proxies 已正确配置。
// 返回 []string：网段字符串列表。
func cfTrustedAll() []string {
	out := make([]string, 0, 24)
	for _, p := range guard.CloudflarePrefixes() {
		out = append(out, p.String())
	}
	return out
}

// TestCDNWarnings 验证 CDN 风险自检的判定规则：
// 橙云站点未信任 CF 网段时必须告警且点名站点；信任网段后不再告警；
// 全直连站点与被禁用站点不产生噪声；多个橙云站点合并为一条告警。
func TestCDNWarnings(t *testing.T) {
	orangeSite := model.Site{
		ID: 1, Name: "橙云站", Hosts: `["a.example.com"]`,
		Upstream: "http://127.0.0.1:8080", Enabled: 1, BehindCDN: "cloudflare",
	}
	graySite := model.Site{
		ID: 2, Name: "灰云站", Hosts: `["b.example.com"]`,
		Upstream: "http://127.0.0.1:8080", Enabled: 1,
	}

	t.Run("橙云站点未信任CF网段必须告警", func(t *testing.T) {
		rt := buildRT(t, []model.Site{orangeSite, graySite}, []string{"127.0.0.1/32", "::1/128"})
		got := rt.CDNWarnings()
		if len(got) != 1 {
			t.Fatalf("告警数 = %d, want 1: %+v", len(got), got)
		}
		if got[0].Code != WarnCDNTrustMissing {
			t.Errorf("code = %q", got[0].Code)
		}
		// 提示里要点名是哪个站点，否则多站点时没法定位
		if !strings.Contains(got[0].Site, "橙云站") {
			t.Errorf("告警未指明站点: %q", got[0].Site)
		}
		if strings.Contains(got[0].Site, "灰云站") {
			t.Errorf("直连站点不该出现在告警里: %q", got[0].Site)
		}
	})

	t.Run("信任CF网段后不再告警", func(t *testing.T) {
		rt := buildRT(t, []model.Site{orangeSite}, cfTrustedAll())
		if got := rt.CDNWarnings(); len(got) != 0 {
			t.Fatalf("不该告警: %+v", got)
		}
	})

	t.Run("全直连站点不产生噪声", func(t *testing.T) {
		rt := buildRT(t, []model.Site{graySite}, []string{"127.0.0.1/32"})
		if got := rt.CDNWarnings(); len(got) != 0 {
			t.Fatalf("不该告警: %+v", got)
		}
	})

	t.Run("站点被禁用时不告警", func(t *testing.T) {
		// 禁用的站点不进路由表，也就不影响取 IP，不该打扰用户
		disabled := orangeSite
		disabled.Enabled = 0
		rt := buildRT(t, []model.Site{disabled}, []string{"127.0.0.1/32"})
		if got := rt.CDNWarnings(); len(got) != 0 {
			t.Fatalf("不该告警: %+v", got)
		}
	})

	t.Run("多个橙云站点合并成一条告警", func(t *testing.T) {
		second := orangeSite
		second.ID = 3
		second.Name = "第二个橙云站"
		second.Hosts = `["c.example.com"]`
		rt := buildRT(t, []model.Site{orangeSite, second}, []string{"127.0.0.1/32"})
		got := rt.CDNWarnings()
		// 一次部署只该弹一条横幅，不能每个站点弹一次
		if len(got) != 1 {
			t.Fatalf("告警数 = %d, want 1", len(got))
		}
		for _, name := range []string{"橙云站", "第二个橙云站"} {
			if !strings.Contains(got[0].Site, name) {
				t.Errorf("告警缺少站点 %q: %q", name, got[0].Site)
			}
		}
	})
}
