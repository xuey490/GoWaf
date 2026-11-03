package guard

import (
	"net/netip"
	"testing"
)

func TestIsCloudflare(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"104.16.1.1", true},   // CF 经典段
		{"172.64.0.1", true},   // CF 段（与 104.16 同族但不同前缀）
		{"2606:4700::1", true}, // IPv6 段
		{"8.8.8.8", false},
		{"127.0.0.1", false},
		{"", false}, // 非法地址不能 panic
	}
	for _, c := range cases {
		var addr netip.Addr
		if c.addr != "" {
			addr = netip.MustParseAddr(c.addr)
		}
		if got := IsCloudflare(addr); got != c.want {
			t.Errorf("IsCloudflare(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

func TestMissingCloudflare(t *testing.T) {
	all := CloudflarePrefixes()
	if len(all) == 0 {
		t.Fatal("内置 CF 网段解析为空")
	}

	t.Run("完全没有信任时全部缺失", func(t *testing.T) {
		// 只剩回环是最常见的错配：装完没执行 cloudflare on
		trusted := ParseTrusted([]string{"127.0.0.1/32", "::1/128"})
		if got := MissingCloudflare(trusted); len(got) != len(all) {
			t.Fatalf("缺失数 = %d, want %d", len(got), len(all))
		}
	})

	t.Run("全量信任时无缺失", func(t *testing.T) {
		raw := make([]string, 0, len(all))
		for _, p := range all {
			raw = append(raw, p.String())
		}
		if got := MissingCloudflare(ParseTrusted(raw)); len(got) != 0 {
			t.Fatalf("仍有缺失: %v", got)
		}
	})

	t.Run("只信任一个段仍算缺失", func(t *testing.T) {
		// 覆盖率语义：只写 104.16.0.0/13 覆盖不到 172.64.0.0/13
		trusted := ParseTrusted([]string{"104.16.0.0/13"})
		got := MissingCloudflare(trusted)
		if len(got) != len(all)-1 {
			t.Fatalf("缺失数 = %d, want %d", len(got), len(all)-1)
		}
		for _, m := range got {
			if m == "104.16.0.0/13" {
				t.Fatal("已信任的段不该出现在缺失列表里")
			}
		}
	})

	t.Run("更大前缀可覆盖多个CF段", func(t *testing.T) {
		// 0.0.0.0/0 应当覆盖全部 IPv4 段，只剩 IPv6 缺失
		got := MissingCloudflare(ParseTrusted([]string{"0.0.0.0/0", "::/0"}))
		if len(got) != 0 {
			t.Fatalf("全放行仍有缺失: %v", got)
		}
	})
}

func TestParseTrusted(t *testing.T) {
	// 单 IP 与 CIDR 混写、以及垃圾值都要能处理：配置是手写 YAML，不能因为一行写错就 panic
	got := ParseTrusted([]string{"127.0.0.1", "10.0.0.0/8", "not-a-cidr", ""})
	if len(got) != 2 {
		t.Fatalf("解析出 %d 条，want 2: %v", len(got), got)
	}
	if got[0] != netip.MustParsePrefix("127.0.0.1/32") {
		t.Errorf("单 IP 未转成 /32 前缀: %v", got[0])
	}
}
