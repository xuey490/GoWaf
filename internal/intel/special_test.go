package intel

import (
	"net/netip"
	"testing"
)

// 公共情报源里混入 bogon 是常态（多来源聚合清单尤其常见）。
// 一旦 127.0.0.0/8 被当成情报，反代没传 X-Forwarded-For 时会把所有请求
// 的 clientIP 退化成 127.0.0.1 —— 整站 403。这里锁死这条路径。
func TestMatchIgnoresSpecialUse(t *testing.T) {
	m := New()
	set := &rangeSet{}
	addToken(set, "127.0.0.0/8")
	addToken(set, "10.0.0.0/8")
	addToken(set, "192.168.1.1")
	addToken(set, "::1")
	addToken(set, "100.64.0.0/10")
	addToken(set, "1.19.0.0/16") // 真实情报，必须保留
	m.global = set
	m.global.compact()

	if got := set.size(); got != 1 {
		t.Fatalf("特殊用途网段应被丢弃，只留 1 条真实情报，实际 %d 条", got)
	}

	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", false},
		{"10.1.2.3", false},
		{"192.168.1.1", false},
		{"::1", false},
		{"100.64.1.1", false},
		{"1.19.0.1", true},
	}
	for _, c := range cases {
		addr := netip.MustParseAddr(c.ip)
		hit := m.Match(addr, "") != ""
		if hit != c.want {
			t.Errorf("%s 命中=%v，期望 %v", c.ip, hit, c.want)
		}
	}
}

// 即使源里真的带着 127.0.0.0/8，匹配层也必须再兜一次（双保险）。
func TestMatchSkipsSpecialUseEvenIfPresent(t *testing.T) {
	m := New()
	set := &rangeSet{}
	set.addPrefix(netip.MustParsePrefix("127.0.0.0/8"))
	m.global = set
	if got := m.Match(netip.MustParseAddr("127.0.0.1"), ""); got != "" {
		t.Fatalf("回环地址不应命中情报，实际返回 %q", got)
	}
}
