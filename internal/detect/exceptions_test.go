package detect

import (
	"testing"

	"gowaf/internal/store/model"
)

// TestExceptionSkipsSelfBuiltRule 验证例外能放过自研规则的命中。
func TestExceptionSkipsSelfBuiltRule(t *testing.T) {
	eng := NewEngine(60, 30, "protect")
	if err := eng.Reload([]model.Rule{{
		ID: "t-sqli", Name: "test sqli", Category: "sqli", Score: 80,
		Targets: "query", Keywords: `["union select"]`, Action: "block", Enabled: 1,
	}}); err != nil {
		t.Fatalf("载入规则失败: %v", err)
	}

	// 无例外时应正常命中（用例前提）
	in := &Input{Method: "GET", Path: "/a", Query: "id=1 union select 2", Site: "s1"}
	if res := eng.Inspect(in); res.Score == 0 {
		t.Fatal("未命中规则，用例前提不成立")
	}

	// 站点+路径+规则 三维匹配
	eng.SetExceptions([]Exception{{Site: "s1", RuleID: "t-sqli", PathPrefix: "/a"}})
	if res := eng.Inspect(in); res.Score != 0 || res.Action != "pass" {
		t.Fatalf("例外未生效: score=%d action=%s", res.Score, res.Action)
	}

	// 规则 ID 不匹配时例外不该生效
	eng.SetExceptions([]Exception{{RuleID: "other-rule"}})
	if res := eng.Inspect(in); res.Score == 0 {
		t.Fatal("例外误伤：规则 ID 不同却被放过")
	}
}

// TestFilterByExceptionOnCRSResult 验证第二引擎结果同样受例外约束。
func TestFilterByExceptionOnCRSResult(t *testing.T) {
	eng := NewEngine(60, 30, "protect")
	eng.SetExceptions([]Exception{{RuleID: "crs-942100"}})

	// 构造带两条命中的 CRS 结果：crs-942100 已在例外中，crs-942190 不在
	res := &Result{
		Score:      100,
		Hits:       []Hit{{RuleID: "crs-942100", Category: "sqli", Score: 60}, {RuleID: "crs-942190", Category: "sqli", Score: 40}},
		RuleIDs:    []string{"crs-942100", "crs-942190"},
		Categories: []string{"sqli"},
	}
	eng.FilterByException(res, "s1", "/", nil)

	if len(res.Hits) != 1 || res.Hits[0].RuleID != "crs-942190" {
		t.Fatalf("过滤结果不符: %+v", res.Hits)
	}
	if res.Score != 40 {
		t.Fatalf("分数未重算: %d", res.Score)
	}
	if len(res.RuleIDs) != 1 || res.RuleIDs[0] != "crs-942190" {
		t.Fatalf("命中规则未重算: %v", res.RuleIDs)
	}

	// 只按路径前缀的例外 = 整个路径放行，CRS 也要一起让开
	eng.SetExceptions([]Exception{{PathPrefix: "/upload"}})
	eng.FilterByException(res, "s1", "/upload/avatar", nil)
	if len(res.Hits) != 0 || res.Score != 0 {
		t.Fatalf("路径级例外未生效: %+v", res)
	}
}
