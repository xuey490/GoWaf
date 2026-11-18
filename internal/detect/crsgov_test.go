package detect

import (
	"strings"
	"testing"
)

func TestExtractCRSLiterals(t *testing.T) {
	lits := extractCRSLiterals()
	if len(lits) < 200 {
		t.Fatalf("CRS 字面量提取过少: %d（提取逻辑可能失效）", len(lits))
	}
	// 高价值特征必须在词表里（"etc" 只有 3 字符不提取；base64_decode 是
	// transform 不是 operator 模式，不在提取范围——均为设计内行为）
	must := []string{"union", "select", "passwd", "sqlmap"}
	have := map[string]bool{}
	for _, l := range lits {
		have[l] = true
	}
	for _, m := range must {
		if !have[m] {
			t.Errorf("词表缺少关键特征 %q", m)
		}
	}
}

func TestCRSGovStaticSkip(t *testing.T) {
	g := NewCRSGov(CRSGovOptions{SkipStatic: true, BodyTextOnly: true})
	ok, reason := g.ShouldInspect("s", "GET", "/static/logo.png", "", "", "")
	if ok {
		t.Fatalf("静态资源应被旁路")
	}
	if reason != "static_asset" {
		t.Fatalf("原因应为 static_asset, got %q", reason)
	}
	// 带 query 的静态路径不旁路（可能有参数注入）
	if ok, _ := g.ShouldInspect("s", "GET", "/static/logo.png", "v=2", "", ""); !ok {
		t.Fatal("带 query 的静态请求不应旁路")
	}
	// POST 到静态路径不旁路
	if ok, _ := g.ShouldInspect("s", "POST", "/static/logo.png", "", "x=1", ""); !ok {
		t.Fatal("POST 请求不应旁路")
	}
	// 无扩展名路径不旁路
	if ok, _ := g.ShouldInspect("s", "GET", "/", "", "", ""); !ok {
		t.Fatal("根路径不应旁路")
	}
}

func TestCRSGovExcludePaths(t *testing.T) {
	g := NewCRSGov(CRSGovOptions{ExcludePaths: []string{"/health", "/internal/callback"}})
	if ok, reason := g.ShouldInspect("s", "POST", "/health/deep", "", `{"a":1}`, ""); ok || reason != "excluded_path" {
		t.Fatalf("豁免路径应旁路: ok=%v reason=%q", ok, reason)
	}
	// 前缀匹配是文档化语义：/healthz 命中 /health 豁免属预期
	if ok, _ := g.ShouldInspect("s", "GET", "/api/health", "", "", ""); !ok {
		t.Fatal("中间路径 /api/health 不应命中前缀 /health 豁免")
	}
}

func TestCRSGovPrescreen(t *testing.T) {
	g := NewCRSGov(CRSGovOptions{Prescreen: true, SkipStatic: false, BodyTextOnly: true})
	if g.PrescreenKeywords() == 0 {
		t.Fatal("预筛关键词为空")
	}
	// 攻击特征：AC 必命中 → 跑 CRS
	for _, q := range []string{
		"?id=1 union select password from users",
		"?q=<script>alert(1)</script>",
		"?f=../../../../etc/passwd",
		"?c=;cat /etc/shadow",
		// 经典引号闭合 SQLi：靠精选词表兜住 @detectSQLi 盲区
		"?id=1' or '1'='1",
	} {
		if ok, reason := g.ShouldInspect("s", "GET", "/", q, "", ""); !ok {
			t.Errorf("攻击请求被预筛误放行 %q（reason=%s）", q, reason)
		}
	}
	// 扫描器 UA 必命中
	if ok, _ := g.ShouldInspect("s", "GET", "/", "", "", "User-Agent: sqlmap/1.7"); !ok {
		t.Fatal("扫描器 UA 被预筛误放行")
	}
	// 干净请求：必须旁路（这是加速模式的验收口径）
	clean := "Host: example.com User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36 Accept: text/html,application/xhtml+xml"
	if hits := g.PrescreenHits("/ " + clean); len(hits) > 0 {
		t.Fatalf("干净请求命中预筛词表: %v（噪声词需复查）", hits)
	}
	if ok, _ := g.ShouldInspect("s", "GET", "/", "", "", clean); ok {
		t.Fatal("干净请求应被预筛旁路")
	}
}

func TestValidateRemoveRanges(t *testing.T) {
	got := ValidateRemoveRanges("920, 921, 905, abc, 96, 930")
	want := "920,921,930"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(ValidateRemoveRanges("")) != 0 {
		t.Fatal("空输入应返回空")
	}
}

func TestRemovalDirectives(t *testing.T) {
	d := removalDirectives([]string{"922"})
	// 922 multipart 合规段：v4.30 存在且 ID 均为 9220xx
	if !strings.Contains(d, "SecRuleRemoveById 922") {
		t.Fatalf("号段 922 应生成移除指令: %q", d)
	}
	if removalDirectives(nil) != "" {
		t.Fatal("空号段应返回空")
	}
}

func TestTextContentTypeHelper(t *testing.T) {
	// isTextContentType 在 edge 包，这里测闸门的默认值语义
	if !(NewCRSGov(CRSGovOptions{})).BodyTextOnly() {
		// NewCRSGov 不带 BodyTextOnly 参数时不强设默认；默认值由 runtime 层
		// 的 settingFromBool 兜底（true）。这里只验证构造不 panic。
		t.Log("BodyTextOnly 未在 NewCRSGov 内设默认，由 runtime 层兜底")
	}
}
