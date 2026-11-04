package guard

import (
	"net/netip"
	"testing"

	"gowaf/internal/store/model"
)

// 站点作用域名单只在该站点命中；全局名单对所有站点生效。
func TestIPListSiteScope(t *testing.T) {
	list := BuildIPList([]model.IPRule{
		{IP: "10.0.0.1", Kind: "black", Scope: "global"},
		{IP: "10.0.0.2", Kind: "black", Scope: "site", Site: "a.com"},
		{IP: "10.0.0.3", Kind: "white", Scope: "site", Site: "b.com"},
		{IP: "10.9.0.0/16", Kind: "black", Scope: "site", Site: "c.com"},
	}, 0)

	addr := func(s string) netip.Addr { return netip.MustParseAddr(s) }

	if !list.IsBlacklisted(addr("10.0.0.1"), "a.com") || !list.IsBlacklisted(addr("10.0.0.1"), "b.com") {
		t.Fatal("全局黑名单应对所有站点生效")
	}
	if !list.IsBlacklisted(addr("10.0.0.2"), "a.com") {
		t.Fatal("站点黑名单应在所属站点生效")
	}
	if list.IsBlacklisted(addr("10.0.0.2"), "b.com") {
		t.Fatal("站点黑名单不应影响其他站点")
	}
	if !list.IsBlacklisted(addr("10.9.1.1"), "c.com") || list.IsBlacklisted(addr("10.9.1.1"), "a.com") {
		t.Fatal("站点 CIDR 黑名单作用域错误")
	}
	if !list.IsWhitelisted(addr("10.0.0.3"), "b.com") {
		t.Fatal("站点白名单应在所属站点生效")
	}
	if list.IsWhitelisted(addr("10.0.0.3"), "a.com") {
		t.Fatal("站点白名单不应影响其他站点")
	}

	white, black := list.Count()
	if white != 1 || black != 3 {
		t.Fatalf("Count = (%d,%d), want (1,3)", white, black)
	}
}

// 脏数据：scope=site 但没有站点，不能当成全局黑名单误伤全站。
func TestIPListSiteScopeMissingSite(t *testing.T) {
	list := BuildIPList([]model.IPRule{
		{IP: "10.0.0.9", Kind: "black", Scope: "site", Site: ""},
	}, 0)
	if list.IsBlacklisted(netip.MustParseAddr("10.0.0.9"), "a.com") {
		t.Fatal("缺少站点的按站点规则应被忽略")
	}
}

func TestBanListSiteScope(t *testing.T) {
	b := NewBanList()
	b.Ban("1.2.3.4", "a.com", 0)

	if !b.Banned("1.2.3.4", "a.com", 0) {
		t.Fatal("站点封禁应在所属站点生效")
	}
	if b.Banned("1.2.3.4", "b.com", 0) {
		t.Fatal("站点封禁不应影响其他站点")
	}

	b.Ban("1.2.3.4", "", 0)
	if !b.Banned("1.2.3.4", "b.com", 0) {
		t.Fatal("全局封禁应对所有站点生效")
	}
	if b.Size() != 1 {
		t.Fatalf("Size = %d, want 1（同一 IP 只算一个）", b.Size())
	}

	// 解站点封禁后全局封禁仍生效
	b.Unban("1.2.3.4", "a.com")
	if !b.Banned("1.2.3.4", "a.com", 0) {
		t.Fatal("全局封禁不应被站点解封带走")
	}
	b.Unban("1.2.3.4", "")
	if b.Banned("1.2.3.4", "a.com", 0) || b.Size() != 0 {
		t.Fatal("解封后不应残留")
	}
}
