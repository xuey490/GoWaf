package guard

import (
	"net/netip"
	"testing"

	"gowaf/internal/store/model"
)

// TestIPListSiteScope 验证名单作用域：全局名单对所有站点生效，
// 站点名单（含精确 IP 与 CIDR）只在该站点命中，并核对 Count 的统计结果。
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

// TestBanListSiteScope 验证封禁表作用域：站点封禁只影响该站点、
// 全局封禁对所有站点生效、两种解封互不影响，Size 对同一 IP 去重计数。
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

// TestIPListWhitePrecedence 验证白名单是「完全豁免」的两个前提：
// 1) 白名单与黑名单同时存在时，白名单判定成立（数据面据此先放行）；
// 2) IsWhitelistedAnywhere 覆盖任意站点作用域，供全局封禁前的校验使用。
func TestIPListWhitePrecedence(t *testing.T) {
	list := BuildIPList([]model.IPRule{
		{IP: "10.0.0.5", Kind: "black", Scope: "global"},
		{IP: "10.0.0.5", Kind: "white", Scope: "global"},
		{IP: "10.0.0.6", Kind: "white", Scope: "site", Site: "a.com"},
	}, 0)

	addr := netip.MustParseAddr("10.0.0.5")
	if !list.IsWhitelisted(addr, "a.com") {
		t.Fatal("黑白名单同时命中时，白名单必须成立（优先于黑名单）")
	}

	// 站点白名单不在该站点时 IsWhitelisted 为 false，但全局封禁前必须能查到
	siteOnly := netip.MustParseAddr("10.0.0.6")
	if list.IsWhitelisted(siteOnly, "b.com") {
		t.Fatal("站点白名单不应在其他站点直接命中")
	}
	if !list.IsWhitelistedAnywhere(siteOnly) {
		t.Fatal("全局封禁前应能查到任意站点作用域的白名单")
	}
	if list.IsWhitelistedAnywhere(netip.MustParseAddr("10.0.0.7")) {
		t.Fatal("未列入白名单的 IP 不应被判定为白名单")
	}
}

// TestIPListWhiteExpiry 验证带到期时间的白名单条目过期后不再生效。
// 若忽略过期时间，「临时放行 2 小时」会变成永久放行，属于安全缺口。
func TestIPListWhiteExpiry(t *testing.T) {
	const now = int64(1_700_000_000_000)
	list := BuildIPList([]model.IPRule{
		{IP: "10.0.0.8", Kind: "white", Scope: "global", ExpiresAt: now - 1},
		{IP: "10.0.0.9", Kind: "white", Scope: "global", ExpiresAt: now + 60_000},
	}, now)

	if list.IsWhitelisted(netip.MustParseAddr("10.0.0.8"), "a.com") {
		t.Fatal("已过期的白名单条目不应再生效")
	}
	if !list.IsWhitelisted(netip.MustParseAddr("10.0.0.9"), "a.com") {
		t.Fatal("未过期的白名单条目必须生效")
	}
}
