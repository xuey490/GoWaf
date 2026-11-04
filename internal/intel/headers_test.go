package intel

import (
	"net/netip"
	"strings"
	"testing"
)

// TestParseHeaders 覆盖两种写法：JSON 对象与 Key: Value 行文本。
func TestParseHeaders(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		key  string
		want string
	}{
		{"json", `{"Key":"abc123","Accept":"text/plain"}`, "Key", "abc123"},
		{"lines", "Key: abc123\nAccept: text/plain", "Key", "abc123"},
		{"带注释与空行", "# 注释\n\nKey: abc123\n", "Key", "abc123"},
		{"值里含冒号", "Authorization: Bearer a:b:c", "Authorization", "Bearer a:b:c"},
		{"空输入", "", "Key", ""},
	}
	for _, c := range cases {
		got := ParseHeaders(c.raw)[c.key]
		if got != c.want {
			t.Errorf("%s: 期望 %q，实际 %q", c.name, c.want, got)
		}
	}
}

// TestParseJSONList AbuseIPDB 的响应结构：{"data":[{"ipAddress":"1.2.3.4"}]}。
func TestParseJSONList(t *testing.T) {
	body := `{"meta":{"generatedAt":"2026-11-04T12:00:00+00:00"},
		"data":[
			{"ipAddress":"5.188.10.179","abuseConfidenceScore":100},
			{"ipAddress":"185.222.209.14","abuseConfidenceScore":98},
			{"ipAddress":"2001:db8::1","abuseConfidenceScore":90}
		]}`
	set, err := parseJSONList(strings.NewReader(body))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := len(set.v4); got != 2 {
		t.Fatalf("IPv4 区间数期望 2，实际 %d", got)
	}
	if got := len(set.v6); got != 1 {
		t.Fatalf("IPv6 前缀数期望 1，实际 %d", got)
	}
}

// TestParseJSONListIgnoresNoise JSON 里混入版本号、时间戳等非 IP 字符串时不应误收。
func TestParseJSONListIgnoresNoise(t *testing.T) {
	body := `{"version":"1.2.3","ts":"2026-11-04","note":"1.0.0-beta","ip":"9.9.9.9"}`
	set, err := parseJSONList(strings.NewReader(body))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := len(set.v4); got != 1 {
		t.Fatalf("期望只收 1 个 IP，实际 %d", got)
	}
	if !set.contains(netip.MustParseAddr("9.9.9.9")) {
		t.Fatal("9.9.9.9 应命中")
	}
}
