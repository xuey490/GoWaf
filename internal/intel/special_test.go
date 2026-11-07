package intel

import (
	"net/netip"
	"testing"
)

// TestMatchIgnoresSpecialUse 验证特殊用途网段（bogon）不会进入情报库，也不会被匹配命中。
// 公共情报源里混入 bogon 是常态（多来源聚合清单尤其常见）。
// 一旦 127.0.0.0/8 被当成情报，反代没传 X-Forwarded-For 时会把所有请求
// 的 clientIP 退化成 127.0.0.1 —— 整站 403。这里锁死这条路径。
func TestMatchIgnoresSpecialUse(t *testing.T) {
	m := New()
	set := &rangeSet{}
	// 通过 addToken 喂入各类 bogon 与一条真实情报（1.19.0.0/16）
	addToken(set, "127.0.0.0/8")
	addToken(set, "10.0.0.0/8")
	addToken(set, "192.168.1.1")
	addToken(set, "::1")
	addToken(set, "100.64.0.0/10")
	addToken(set, "1.19.0.0/16") // 真实情报，必须保留
	m.global = set
	m.global.compact()

	// 数据层过滤：bogon 全部丢弃，只留真实情报
	if got := set.size(); got != 1 {
		t.Fatalf("特殊用途网段应被丢弃，只留 1 条真实情报，实际 %d 条", got)
	}

	// 匹配层校验：特殊用途地址一律不命中，真实情报地址必须命中
	cases := []struct {
		ip   string // 待检查的地址
		want bool   // 期望是否命中
	}{
		{"127.0.0.1", false},   // 回环地址
		{"10.1.2.3", false},    // 私网地址
		{"192.168.1.1", false}, // 私网地址（单 IP 形式）
		{"::1", false},         // IPv6 回环
		{"100.64.1.1", false},  // 运营商级 NAT（CGNAT）
		{"1.19.0.1", true},     // 真实情报，必须命中
	}
	for _, c := range cases {
		addr := netip.MustParseAddr(c.ip)
		hit := m.Match(addr, "") != ""
		if hit != c.want {
			t.Errorf("%s 命中=%v，期望 %v", c.ip, hit, c.want)
		}
	}
}

// TestMatchSkipsSpecialUseEvenIfPresent 验证匹配层的双保险：
// 即使数据层漏进了 127.0.0.0/8（绕过 addToken 的过滤、直接 addPrefix 塞入），
// Match 也必须拒绝命中回环地址。
func TestMatchSkipsSpecialUseEvenIfPresent(t *testing.T) {
	m := New()
	set := &rangeSet{}
	// 不经过 addToken，直接把 bogon 塞进集合，模拟脏数据
	set.addPrefix(netip.MustParsePrefix("127.0.0.0/8"))
	m.global = set
	if got := m.Match(netip.MustParseAddr("127.0.0.1"), ""); got != "" {
		t.Fatalf("回环地址不应命中情报，实际返回 %q", got)
	}
}
