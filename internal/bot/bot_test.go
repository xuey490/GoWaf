package bot

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

// ipOf 把字符串转成 netip.Addr。
func ipOf(s string) netip.Addr { return netip.MustParseAddr(s) }

// TestCheckDisabledByDefault 验证默认关闭：任何请求都返回未命中。
func TestCheckDisabledByDefault(t *testing.T) {
	m := NewManager()
	v := m.Check(&Request{IP: ipOf("1.2.3.4"), UA: "curl/8.0", Path: "/backup.zip"})
	if v.Matched {
		t.Fatalf("未启用时不应命中: %+v", v)
	}
	// nil 请求同样安全
	if v := m.Check(nil); v.Matched {
		t.Fatal("nil 请求不应命中")
	}
}

// TestUAPolicy 验证 UA 策略命中与动作透传。
func TestUAPolicy(t *testing.T) {
	m := NewManager()
	err := m.Reload([]PolicyDef{
		{ID: 1, Name: "封curl", Category: "scanner", Action: ActionBlock, MatchType: "ua", Pattern: "curl", Priority: 1, Enabled: true},
	}, nil, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}

	v := m.Check(&Request{IP: ipOf("1.2.3.4"), UA: "curl/8.1.0", Path: "/a"})
	if !v.Matched || v.Action != ActionBlock || v.Category != "scanner" || v.BotName != "封curl" || v.Score != 60 {
		t.Fatalf("UA 策略命中结果异常: %+v", v)
	}
	if !strings.Contains(v.Reason, "封curl") {
		t.Fatalf("Reason 应含策略名: %q", v.Reason)
	}

	// UA 不含 curl：不命中该策略
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), UA: "Mozilla/5.0", Path: "/a"}); v.Matched {
		t.Fatalf("正常 UA 不该命中: %+v", v)
	}

	// 正则忽略大小写：Reload 统一加 (?i)
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), UA: "CURL/8.1.0", Path: "/a"}); !v.Matched {
		t.Fatal("UA 匹配应忽略大小写")
	}
}

// TestPathAndIPPolicies 验证 path 正则与 IP/CIDR 策略。
func TestPathAndIPPolicies(t *testing.T) {
	m := NewManager()
	err := m.Reload([]PolicyDef{
		{ID: 1, Name: "扫后台", Action: ActionChallenge, MatchType: "path", Pattern: `^/(wp-admin|phpmyadmin)`, Priority: 1, Enabled: true},
		{ID: 2, Name: "内网扫描器", Action: ActionLog, MatchType: "ip", Pattern: "10.0.0.0/8", Priority: 2, Enabled: true},
	}, nil, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}

	if v := m.Check(&Request{IP: ipOf("192.168.1.1"), Path: "/wp-admin/setup"}); !v.Matched || v.Action != ActionChallenge {
		t.Fatalf("path 策略应命中: %+v", v)
	}
	if v := m.Check(&Request{IP: ipOf("10.1.2.3"), Path: "/normal"}); !v.Matched || v.Action != ActionLog {
		t.Fatalf("ip 策略应命中: %+v", v)
	}
	if v := m.Check(&Request{IP: ipOf("11.1.2.3"), Path: "/normal"}); v.Matched {
		t.Fatalf("网段外 IP 不该命中: %+v", v)
	}
	// 单个 IP 自动补 /32
	err = m.Reload([]PolicyDef{
		{ID: 1, Name: "封单IP", Action: ActionBlock, MatchType: "ip", Pattern: "6.6.6.6", Enabled: true},
	}, nil, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("单 IP Reload 失败: %v", err)
	}
	if v := m.Check(&Request{IP: ipOf("6.6.6.6"), Path: "/"}); !v.Matched {
		t.Fatal("单 IP 策略应命中")
	}
}

// TestHoneypot 验证蜜罐路径：精确与通配前缀，命中即 ban。
func TestHoneypot(t *testing.T) {
	m := NewManager()
	err := m.Reload(nil, []HoneypotDef{
		{ID: 1, Path: "/secret-panel", Enabled: true},
		{ID: 2, Path: "/backup*", Enabled: true},
		{ID: 3, Path: "/disabled", Enabled: false},
	}, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}

	// 精确命中
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Path: "/secret-panel"}); !v.Matched ||
		v.Action != ActionBan || v.Category != "honeypot" || v.Score != 100 {
		t.Fatalf("蜜罐精确命中异常: %+v", v)
	}
	// 通配前缀命中
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Path: "/backup.sql"}); !v.Matched || v.Action != ActionBan {
		t.Fatalf("蜜罐前缀命中异常: %+v", v)
	}
	// 蜜罐只在精确路径命中：/secret-panel-x 不该中精确规则
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Path: "/secret-panel-x"}); v.Matched {
		t.Fatalf("非蜜罐路径不该命中: %+v", v)
	}
	// 禁用的蜜罐不生效
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Path: "/disabled"}); v.Matched {
		t.Fatal("禁用蜜罐不该命中")
	}
	// 蜜罐优先于显式策略
	err = m.Reload([]PolicyDef{
		{ID: 9, Name: "允许一切", Action: ActionAllow, MatchType: "path", Pattern: ".", Enabled: true},
	}, []HoneypotDef{{ID: 1, Path: "/secret-panel", Enabled: true}}, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Path: "/secret-panel"}); !v.Matched || v.Category != "honeypot" {
		t.Fatalf("蜜罐应优先于显式策略: %+v", v)
	}
}

// TestSiteScoping 验证站点维度隔离。
func TestSiteScoping(t *testing.T) {
	m := NewManager()
	err := m.Reload([]PolicyDef{
		{ID: 1, Name: "shop UA", Site: "shop", Action: ActionBlock, MatchType: "ua", Pattern: "curl", Enabled: true},
	}, []HoneypotDef{{ID: 2, Site: "blog", Path: "/trap", Enabled: true}},
		Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	// shop 的 UA 策略对 blog 不生效
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Site: "blog", UA: "curl/1", Path: "/a"}); v.Matched {
		t.Fatalf("站点隔离失效: %+v", v)
	}
	// blog 的蜜罐对 shop 不生效
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Site: "shop", Path: "/trap"}); v.Matched {
		t.Fatalf("蜜罐站点隔离失效: %+v", v)
	}
	// 空站点策略全局生效
	err = m.Reload([]PolicyDef{
		{ID: 1, Name: "全局", Action: ActionBlock, MatchType: "ua", Pattern: "curl", Enabled: true},
	}, nil, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), Site: "任意站", UA: "curl/1", Path: "/a"}); !v.Matched {
		t.Fatal("空站点策略应全局生效")
	}
}

// TestReloadRejectsInvalid 验证非法配置整体拒绝且旧快照保留。
func TestReloadRejectsInvalid(t *testing.T) {
	m := NewManager()
	// 先载入合法快照
	if err := m.Reload([]PolicyDef{
		{ID: 1, Name: "旧策略", Action: ActionBlock, MatchType: "ua", Pattern: "curl", Enabled: true},
	}, nil, Settings{Enabled: true}); err != nil {
		t.Fatalf("首次 Reload 失败: %v", err)
	}

	bad := []struct {
		name string
		defs []PolicyDef
		pots []HoneypotDef
		st   Settings
	}{
		{name: "非法动作", defs: []PolicyDef{{ID: 1, MatchType: "ua", Pattern: "x", Action: "nuke", Enabled: true}}},
		{name: "非法matchType", defs: []PolicyDef{{ID: 1, MatchType: "referer", Pattern: "x", Action: ActionBlock, Enabled: true}}},
		{name: "非法正则", defs: []PolicyDef{{ID: 1, MatchType: "ua", Pattern: "([a-", Action: ActionBlock, Enabled: true}}},
		{name: "非法CIDR", defs: []PolicyDef{{ID: 1, MatchType: "ip", Pattern: "999.1.1.1", Action: ActionBlock, Enabled: true}}},
	}
	for _, tc := range bad {
		if err := m.Reload(tc.defs, tc.pots, tc.st); err == nil {
			t.Errorf("%s: 应 Reload 失败", tc.name)
		}
	}

	// 失败后旧快照仍然生效
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), UA: "curl/1", Path: "/a"}); !v.Matched || v.BotName != "旧策略" {
		t.Fatalf("编译失败后旧快照应保留: %+v", v)
	}
}

// TestDisabledPoliciesSkipped 验证 Enabled=false 的策略与蜜罐被跳过。
func TestDisabledPoliciesSkipped(t *testing.T) {
	m := NewManager()
	err := m.Reload([]PolicyDef{
		{ID: 1, Name: "关着的", Action: ActionBlock, MatchType: "ua", Pattern: "curl", Enabled: false},
	}, []HoneypotDef{{ID: 2, Path: "/trap", Enabled: false}}, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	p, h := m.Stats()
	if p != 0 || h != 0 {
		t.Fatalf("禁用条目不应编入快照: policies=%d honeypots=%d", p, h)
	}
	if v := m.Check(&Request{IP: ipOf("1.2.3.4"), UA: "curl/1", Path: "/trap"}); v.Matched {
		t.Fatal("禁用条目不该命中")
	}
}

// TestPriorityOrdering 验证策略按优先级升序，同级按 ID 升序。
func TestPriorityOrdering(t *testing.T) {
	m := NewManager()
	err := m.Reload([]PolicyDef{
		{ID: 10, Name: "低优", Action: ActionLog, MatchType: "ua", Pattern: "curl", Priority: 10, Enabled: true},
		{ID: 5, Name: "高优", Action: ActionBlock, MatchType: "ua", Pattern: "curl", Priority: 1, Enabled: true},
	}, nil, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	v := m.Check(&Request{IP: ipOf("1.2.3.4"), UA: "curl/1", Path: "/a"})
	if v.BotName != "高优" || v.Action != ActionBlock {
		t.Fatalf("优先级 1 应先生效: %+v", v)
	}
}

// TestBehaviorScoring 验证行为评分：窗口计数达到阈值触发，可疑 UA 加分。
func TestBehaviorScoring(t *testing.T) {
	m := NewManager()
	err := m.Reload(nil, nil, Settings{Enabled: true, ScoreThreshold: 3, Action: ActionChallenge})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	req := &Request{IP: ipOf("2.2.2.2"), UA: "Mozilla/5.0", Path: "/"}

	// 前两次未达阈值：不命中
	for i := 0; i < 2; i++ {
		if v := m.Check(req); v.Matched {
			t.Fatalf("第 %d 次不该命中: %+v", i+1, v)
		}
	}
	// 第三次达到阈值：命中，动作取设置值（默认 challenge）
	v := m.Check(req)
	if !v.Matched || v.Action != ActionChallenge || v.Category != "unknown" {
		t.Fatalf("达到阈值应命中: %+v", v)
	}
	if v.Score < 60 {
		t.Fatalf("评分应至少 60: %d", v.Score)
	}
	if !strings.Contains(v.Reason, "3 次") {
		t.Fatalf("Reason 应含请求次数: %q", v.Reason)
	}

	// 可疑 UA 单次请求只有 30 分：不命中
	m2 := NewManager()
	if err := m2.Reload(nil, nil, Settings{Enabled: true, ScoreThreshold: 3, Action: ActionBlock}); err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if v := m2.Check(&Request{IP: ipOf("3.3.3.3"), UA: "python-requests/2.0", Path: "/"}); v.Matched {
		t.Fatalf("单次可疑 UA 不该直接命中: %+v", v)
	}
	// 计数低于阈值时评分仍是 30：不命中
	if v := m2.Check(&Request{IP: ipOf("3.3.3.3"), UA: "python-requests/2.0", Path: "/"}); v.Matched {
		t.Fatalf("低于阈值不该命中: %+v", v)
	}
	// 第 3 次计数达到阈值：60 + 30 = 90 分命中
	if v := m2.Check(&Request{IP: ipOf("3.3.3.3"), UA: "python-requests/2.0", Path: "/"}); !v.Matched || v.Score != 90 {
		t.Fatalf("可疑 UA + 频次应命中 90 分: %+v", v)
	}
}

// TestScoringSkippedForInvalidIP 验证无效 IP 不崩溃且走评分路径。
func TestScoringSkippedForInvalidIP(t *testing.T) {
	m := NewManager()
	err := m.Reload(nil, nil, Settings{Enabled: true, VerifyRDNS: true, ScoreThreshold: 1})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	// 零值 IP：rDNS 验证跳过，空 UA 可疑 +30 不够，但阈值 1 时计数 1 = 60 分命中
	v := m.Check(&Request{UA: "", Path: "/a"})
	if !v.Matched || v.Action != ActionChallenge {
		t.Fatalf("无效 IP + 空UA + 阈值1 应命中: %+v", v)
	}
}

// TestSearchEngineIdentityTable 验证 UA → 搜索引擎识别表。
func TestSearchEngineIdentityTable(t *testing.T) {
	cases := []struct {
		ua     string
		name   string
		suffix string
	}{
		{"Mozilla/5.0 (compatible; Googlebot/2.1)", "Googlebot", ".googlebot.com"},
		{"Mozilla/5.0 (compatible; bingbot/2.0)", "Bingbot", ".search.msn.com"},
		{"Baiduspider-image", "Baiduspider", ".baidu.com"},
		{"Mozilla/5.0 (compatible; YandexBot/3.0)", "YandexBot", ".yandex.com"},
		{"Applebot/1.0", "Applebot", ".applebot.apple.com"},
		{"PetalBot", "PetalBot", ".petalsearch.com"},
		{"Google-InspectionTool/1.0", "Google-InspectionTool", ".google.com"},
	}
	for _, tc := range cases {
		name, suffix := botIdentity(tc.ua)
		if name != tc.name || suffix != tc.suffix {
			t.Errorf("botIdentity(%q) = (%q,%q), 期望 (%q,%q)", tc.ua, name, suffix, tc.name, tc.suffix)
		}
	}
	if name, _ := botIdentity("Mozilla/5.0"); name != "" {
		t.Errorf("普通浏览器 UA 不该识别为 Bot: %q", name)
	}
	// 识别忽略大小写
	if name, _ := botIdentity("GoOgL eBoT demo"); !strings.EqualFold(name, "Googlebot") {
		// UA 里没有连续 googlebot 字样，应识别失败
		if name != "" {
			t.Errorf("非连续字样不该命中: %q", name)
		}
	}
	if name, _ := botIdentity("MyGooglebotChecker"); name != "Googlebot" {
		t.Errorf("包含 googlebot 字样应命中: %q", name)
	}
}

// TestSuspiciousUA 验证可疑 UA 判定。
func TestSuspiciousUA(t *testing.T) {
	suspicious := []string{
		"curl/8.1.0", "Python-Requests/2.31", "Wget/1.21", "Go-http-client/2.0",
		"Scrapy/2.10", "HeadlessChrome/120", "phantomjs", "Selenium", "okhttp/4.9",
		"Java/11.0.2", "", "   ",
	}
	for _, ua := range suspicious {
		if !suspiciousUA(ua) {
			t.Errorf("UA %q 应判定可疑", ua)
		}
	}
	normal := []string{"Mozilla/5.0 (Windows NT 10.0)", "Safari/605.1.15", "Edg/120.0"}
	for _, ua := range normal {
		if suspiciousUA(ua) {
			t.Errorf("UA %q 不该判定可疑", ua)
		}
	}
}

// TestSettingsDefaults 验证 Reload 补全缺省设置。
func TestSettingsDefaults(t *testing.T) {
	m := NewManager()
	if err := m.Reload(nil, nil, Settings{Enabled: true}); err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	st := m.Settings()
	if st.Window != time.Minute {
		t.Fatalf("Window 缺省应为 1 分钟: %v", st.Window)
	}
	if st.CacheTTL != 10*time.Minute {
		t.Fatalf("CacheTTL 缺省应为 10 分钟: %v", st.CacheTTL)
	}
	if st.ScoreThreshold != 30 {
		t.Fatalf("ScoreThreshold 缺省应为 30: %d", st.ScoreThreshold)
	}
	if st.Action != ActionChallenge {
		t.Fatalf("Action 缺省应为 challenge: %s", st.Action)
	}
}

// TestCleanupNoPanic 验证清理入口可安全调用。
func TestCleanupNoPanic(t *testing.T) {
	m := NewManager()
	m.Cleanup(time.Now())
}

// TestPolicyPrioritySortedInStats 验证 Stats 计数。
func TestStatsCount(t *testing.T) {
	m := NewManager()
	err := m.Reload([]PolicyDef{
		{ID: 1, MatchType: "ua", Pattern: "a", Action: ActionLog, Enabled: true},
		{ID: 2, MatchType: "path", Pattern: "b", Action: ActionLog, Enabled: true},
	}, []HoneypotDef{{ID: 3, Path: "/t", Enabled: true}}, Settings{Enabled: true})
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	p, h := m.Stats()
	if p != 2 || h != 1 {
		t.Fatalf("Stats = (%d,%d), 期望 (2,1)", p, h)
	}
}
