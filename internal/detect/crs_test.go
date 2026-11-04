package detect

import (
	"strings"
	"testing"
)

func TestCRSDetectsCommonAttacks(t *testing.T) {
	eng, err := NewCRSEngine(CRSOptions{Mode: "block", Paranoia: 1})
	if err != nil {
		t.Fatalf("加载 CRS 失败: %v", err)
	}

	cases := []struct {
		name string
		in   Input
		want string
	}{
		{
			name: "SQL 注入",
			in: Input{
				Method: "GET", Host: "example.com", Path: "/index.php",
				Query: "id=1%20union%20select%201,2,3",
				UA:    "Mozilla/5.0",
			},
			want: "crs-942",
		},
		{
			name: "扫描器 UA",
			in: Input{
				Method: "GET", Host: "example.com", Path: "/",
				UA: "sqlmap/1.7",
			},
			want: "crs-913",
		},
		{
			name: "敏感文件",
			in: Input{
				Method: "GET", Host: "example.com", Path: "/.env",
				UA: "Mozilla/5.0",
			},
			want: "crs-930",
		},
	}

	for _, c := range cases {
		res := eng.Inspect(&c.in)
		if res.Score == 0 {
			t.Errorf("%s: 期望命中，实际 0 分", c.name)
			continue
		}
		found := false
		for _, id := range res.RuleIDs {
			if strings.HasPrefix(id, c.want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: 未命中期望的规则族 %s，实际 %v", c.name, c.want, res.RuleIDs)
		}
	}
}

func TestCRSNormalRequestLowScore(t *testing.T) {
	eng, err := NewCRSEngine(CRSOptions{Mode: "block", Paranoia: 1})
	if err != nil {
		t.Fatalf("加载 CRS 失败: %v", err)
	}
	in := Input{
		Method: "GET", Host: "www.example.com", Path: "/article/123",
		Query: "page=2&sort=desc",
		UA:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120 Safari/537.36",
	}
	res := eng.Inspect(&in)
	if res.Score >= 60 {
		t.Fatalf("正常请求不该达到拦截阈值，score=%d hits=%v", res.Score, res.RuleIDs)
	}
}

func TestCRSPanicSafe(t *testing.T) {
	var eng *CRSEngine
	res := eng.Inspect(&Input{})
	if res.Score != 0 || len(res.Hits) != 0 {
		t.Fatalf("nil 引擎应返回空结果")
	}
}

// TestCRSVersionV430 锁定内嵌 CRS 版本为 v4.30.0（升级打包子模块时同步更新）。
func TestCRSVersionV430(t *testing.T) {
	eng, err := NewCRSEngine(CRSOptions{Mode: "block", Paranoia: 1})
	if err != nil {
		t.Fatalf("加载 CRS 失败: %v", err)
	}
	if got := eng.Version(); got != "crs-v4.30.0" {
		t.Fatalf("CRS 版本不符: got %q, want crs-v4.30.0", got)
	}
}

// TestSecLangPackageFlow 验证 SecLang 规则包：校验 → 编译进引擎 → 命中透出。
func TestSecLangPackageFlow(t *testing.T) {
	content := `# 测试规则包
SecRule ARGS:token "@rx evilpack" "id:1001234,phase:2,deny,log,msg:'SecLang pack rule',severity:'CRITICAL'"
`
	n, err := ValidateSecLang(content)
	if err != nil {
		t.Fatalf("SecLang 校验失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("规则条数不符: got %d, want 1", n)
	}
	// 引擎全局开关必须被拒
	if _, err := ValidateSecLang("SecRuleEngine On\n" + content); err == nil {
		t.Fatal("包含 SecRuleEngine 的包应该被拒绝")
	}
	// 空/非 SecLang 内容必须被拒
	if _, err := ValidateSecLang(""); err == nil {
		t.Fatal("空包应该被拒绝")
	}
	if _, err := ValidateSecLang("this is not seclang at all"); err == nil {
		t.Fatal("非 SecLang 内容应该编译失败")
	}

	eng, err := NewCRSEngine(CRSOptions{
		Mode: "block", Paranoia: 1,
		Packages: []CRSPackage{{ID: "test", Content: content}},
	})
	if err != nil {
		t.Fatalf("加载 CRS+包失败: %v", err)
	}
	if eng.Packages() != 1 {
		t.Fatalf("包数量不符: %d", eng.Packages())
	}
	// 命中包规则
	res := eng.Inspect(&Input{
		Method: "GET", Host: "example.com", Path: "/",
		Query: "token=evilpack", UA: "Mozilla/5.0",
	})
	found := false
	for _, h := range res.Hits {
		if h.RuleID == "seclang-1001234" {
			found = true
			if h.Category != "seclang" {
				t.Fatalf("包规则分类不符: %s", h.Category)
			}
		}
	}
	if !found {
		t.Fatalf("包规则未命中, hits=%v", res.RuleIDs)
	}
	if res.Score < 40 {
		t.Fatalf("包规则未计分: %d", res.Score)
	}
	// 正常请求不受影响
	ok := eng.Inspect(&Input{Method: "GET", Host: "example.com", Path: "/", UA: "Mozilla/5.0"})
	if ok.Score >= 30 && ok.Action != "block" {
		t.Fatalf("正常请求误拦: score=%d", ok.Score)
	}
	if ok.Action == "block" {
		t.Fatalf("正常请求被包规则误拦: hits=%v", ok.RuleIDs)
	}
}
