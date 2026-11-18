package store_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"gowaf/internal/config"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// newSeedStoreForTest 打开一个临时库（迁移由 Open 自动执行）。
func newSeedStoreForTest(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "waf.db"))
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// listSitesForTest 读出全部站点（按 id 升序）。
func listSitesForTest(t *testing.T, st *store.Store) []model.Site {
	t.Helper()
	var rows []model.Site
	if err := st.Gorm().Order("id asc").Find(&rows).Error; err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}
	return rows
}

// hostsOfForTest 解析站点 hosts 字段。
func hostsOfForTest(t *testing.T, raw string) []string {
	t.Helper()
	var hosts []string
	if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
		t.Fatalf("hosts 解析失败: %v", err)
	}
	return hosts
}

// TestSeedSitesSameNameIdempotent 同名种子重复写入不应产生多行。
func TestSeedSitesSameNameIdempotent(t *testing.T) {
	st := newSeedStoreForTest(t)
	seeds := []config.SiteSeed{
		{Name: "fast.example.com", Hosts: []string{"fast.example.com"}, Upstream: "http://127.0.0.1:8080"},
	}
	for i := 0; i < 3; i++ {
		if err := st.SeedSites(seeds); err != nil {
			t.Fatalf("第 %d 次 SeedSites 失败: %v", i+1, err)
		}
	}
	rows := listSitesForTest(t, st)
	if len(rows) != 1 {
		t.Fatalf("同名种子应只有 1 行，实际 %d 行", len(rows))
	}
}

// TestSeedSitesMergesWhenPrimaryNameChanges 复现并守住真实故障：
// vhost 主域名从裸域名变成宝塔 hash 前缀域名后重装，配置里的站点名随之改变。
// 期望：并入既有站点（补别名），而不是再插入一条重复站点。
func TestSeedSitesMergesWhenPrimaryNameChanges(t *testing.T) {
	st := newSeedStoreForTest(t)

	// 第一次部署：vhost ServerName 是裸域名
	first := []config.SiteSeed{
		{Name: "fast.phpframe.org", Hosts: []string{"fast.phpframe.org"}, Upstream: "http://127.0.0.1:8080"},
	}
	if err := st.SeedSites(first); err != nil {
		t.Fatalf("首次 SeedSites 失败: %v", err)
	}

	// 第二次部署：宝塔把 hash 域名设为 ServerName，裸域名退为 ServerAlias
	second := []config.SiteSeed{
		{Name: "1ddf79d3.fast.phpframe.org",
			Hosts:    []string{"1ddf79d3.fast.phpframe.org", "fast.phpframe.org"},
			Upstream: "http://127.0.0.1:8080"},
	}
	if err := st.SeedSites(second); err != nil {
		t.Fatalf("二次 SeedSites 失败: %v", err)
	}

	rows := listSitesForTest(t, st)
	if len(rows) != 1 {
		t.Fatalf("主域名变更后应仍为 1 行（合并别名），实际 %d 行", len(rows))
	}
	// 站点名保持首次的裸域名，不因配置改名而重命名（避免打断例外规则/策略的引用）
	if rows[0].Name != "fast.phpframe.org" {
		t.Fatalf("站点名不应被种子改写，实际 %q", rows[0].Name)
	}
	hosts := hostsOfForTest(t, rows[0].Hosts)
	if len(hosts) != 2 || hosts[0] != "fast.phpframe.org" || hosts[1] != "1ddf79d3.fast.phpframe.org" {
		t.Fatalf("别名应并入既有 hosts，实际 %v", hosts)
	}

	// 再次重装（配置不变）必须完全幂等
	if err := st.SeedSites(second); err != nil {
		t.Fatalf("三次 SeedSites 失败: %v", err)
	}
	if rows := listSitesForTest(t, st); len(rows) != 1 || rows[0].Hosts != `["fast.phpframe.org","1ddf79d3.fast.phpframe.org"]` {
		t.Fatalf("重复重装应保持 1 行且 hosts 不变，实际 %d 行 %q", len(rows), rows[0].Hosts)
	}
}

// TestSeedSitesWildcardStaysSeparate 兜底站点（hosts=["*"]）不能与业务站点互相吞并。
func TestSeedSitesWildcardStaysSeparate(t *testing.T) {
	st := newSeedStoreForTest(t)
	seeds := []config.SiteSeed{
		{Name: "a.example.com", Hosts: []string{"a.example.com"}, Upstream: "http://127.0.0.1:8080"},
		{Name: "兜底站点", Hosts: []string{"*"}, Upstream: "http://127.0.0.1:8080"},
	}
	if err := st.SeedSites(seeds); err != nil {
		t.Fatalf("SeedSites 失败: %v", err)
	}
	if err := st.SeedSites(seeds); err != nil {
		t.Fatalf("重复 SeedSites 失败: %v", err)
	}
	rows := listSitesForTest(t, st)
	if len(rows) != 2 {
		t.Fatalf("业务站点与兜底站点应各 1 行，实际 %d 行", len(rows))
	}
	for _, r := range rows {
		if r.Name == "兜底站点" {
			if hosts := hostsOfForTest(t, r.Hosts); len(hosts) != 1 || hosts[0] != "*" {
				t.Fatalf("兜底站点 hosts 不应被改写，实际 %v", hosts)
			}
		}
	}
}
