package intel

import (
	"net/netip"
	"strings"
	"testing"
)

// TestParseHeaders 验证自定义请求头解析的两种写法与容错行为：
// JSON 对象、Key: Value 行文本、注释行与空行、值中含冒号、空输入。
func TestParseHeaders(t *testing.T) {
	cases := []struct {
		name string // 用例名称，失败时用于定位
		raw  string // 输入的原始请求头配置
		key  string // 待校验的请求头名
		want string // 期望的请求头值
	}{
		{"json", `{"Key":"abc123","Accept":"text/plain"}`, "Key", "abc123"},       // JSON 对象写法
		{"lines", "Key: abc123\nAccept: text/plain", "Key", "abc123"},             // 行文本写法
		{"带注释与空行", "# 注释\n\nKey: abc123\n", "Key", "abc123"},                      // 注释行与空行应被跳过
		{"值里含冒号", "Authorization: Bearer a:b:c", "Authorization", "Bearer a:b:c"}, // 只按第一个冒号切分，值里的冒号保留
		{"空输入", "", "Key", ""},                                                    // 空输入返回空映射，取值为空串
	}
	for _, c := range cases {
		got := ParseHeaders(c.raw)[c.key]
		if got != c.want {
			t.Errorf("%s: 期望 %q，实际 %q", c.name, c.want, got)
		}
	}
}

// TestParseJSONList 验证 AbuseIPDB 风格响应（{"data":[{"ipAddress":"1.2.3.4"}]}）
// 的解析：递归遍历应把 data 数组里的两个 IPv4 与一个 IPv6 地址全部收进来，
// meta 对象里的时间戳字符串不应被误收。
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
	// 两个 IPv4 地址各展开为一个 /32 区间，互不重叠，共 2 条
	if got := len(set.v4); got != 2 {
		t.Fatalf("IPv4 区间数期望 2，实际 %d", got)
	}
	// IPv6 地址收为 /128 前缀
	if got := len(set.v6); got != 1 {
		t.Fatalf("IPv6 前缀数期望 1，实际 %d", got)
	}
}

// TestParseJSONListIgnoresNoise 验证 JSON 容错收集的降噪：版本号（1.2.3）、
// 日期（2026-11-04）、带后缀的版本串（1.0.0-beta）都不是合法 IP，不应误收；
// 只有真正的地址字段（9.9.9.9）进入情报库。
func TestParseJSONListIgnoresNoise(t *testing.T) {
	body := `{"version":"1.2.3","ts":"2026-11-04","note":"1.0.0-beta","ip":"9.9.9.9"}`
	set, err := parseJSONList(strings.NewReader(body))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	// 噪声字符串被 addToken 静默丢弃，仅 9.9.9.9 入库
	if got := len(set.v4); got != 1 {
		t.Fatalf("期望只收 1 个 IP，实际 %d", got)
	}
	if !set.contains(netip.MustParseAddr("9.9.9.9")) {
		t.Fatal("9.9.9.9 应命中")
	}
}
