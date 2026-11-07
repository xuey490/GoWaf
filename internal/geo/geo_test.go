package geo

import (
	"net/netip"
	"strings"
	"testing"
)

// TestLookupKnownIPs 验证 LookupString 的核心查询路径：
// 回环/内网/IPv6 的特殊标注；公网 IP 能查到非空归属地；
// 并抽查 114.114.114.114 应带「中国」前缀、8.8.8.8 应为「美国」。
// 数据库未打包时跳过。
func TestLookupKnownIPs(t *testing.T) {
	d := Default()
	if !d.Ready() {
		t.Skip("归属地库未打包")
	}
	cases := []struct {
		ip   string
		want string
	}{
		{"127.0.0.1", "本机"},
		{"192.168.1.1", "内网"},
		{"10.1.2.3", "内网"},
		{"2606:4700::1", "IPv6"},
	}
	for _, c := range cases {
		got := d.LookupString(c.ip)
		if got != c.want {
			t.Errorf("%s => %q, want %q", c.ip, got, c.want)
		}
	}

	// 公网地址只断言非空且不是内网标记，避免数据更新导致测试脆断
	for _, ip := range []string{"1.0.1.5", "8.8.8.8", "114.114.114.114"} {
		got := d.LookupString(ip)
		if got == "" || got == "内网" || got == "本机" {
			t.Errorf("%s => %q，期望能查到归属地", ip, got)
		}
	}
	if got := d.LookupString("114.114.114.114"); !strings.HasPrefix(got, "中国") {
		t.Errorf("国内 IP 应返回中国前缀，got %q", got)
	}
	if got := d.LookupString("8.8.8.8"); got != "美国" {
		t.Errorf("8.8.8.8 应为美国，got %q", got)
	}
}

// TestNormalizeRegion 验证国家/省/市字段的归一化规则：
// 国内省市后缀裁剪与去重、港澳台专名、海外英文国名翻译、
// 脏数据（"0"、未知国家）的兜底输出。
func TestNormalizeRegion(t *testing.T) {
	cases := []struct{ country, province, city, want string }{
		{"中国", "广东省", "深圳市", "中国 广东 深圳"},
		{"中国", "香港特别行政区", "0", "中国香港"},
		{"中国", "台湾省", "台北市", "中国台湾 台北"},
		{"中国", "北京市", "北京市", "中国 北京"},
		{"United States", "California", "San Jose", "美国"},
		{"Japan", "0", "0", "日本"},
		{"Atlantis", "0", "0", "Atlantis"},
		{"0", "0", "0", "未知"},
	}
	for _, c := range cases {
		if got := NormalizeRegion(c.country, c.province, c.city); got != c.want {
			t.Errorf("NormalizeRegion(%q,%q,%q) = %q, want %q", c.country, c.province, c.city, got, c.want)
		}
	}
}

// BenchmarkLookup 测量共享实例上单次 Lookup 的耗时（二分查找路径）。
func BenchmarkLookup(b *testing.B) {
	d := Default()
	addr := netip.MustParseAddr("114.114.114.114")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Lookup(addr)
	}
}
