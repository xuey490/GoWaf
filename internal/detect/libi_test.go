package detect

import "testing"

// TestLibiInspectSQLi 验证 libinjection 通道对典型 SQLi payload 的识别与计分。
func TestLibiInspectSQLi(t *testing.T) {
	e := NewLibiEngine(true, DefaultLibiSQLiScore, DefaultLibiXSSScore)
	// 解码后的载荷（URL 编码由归一化层处理，libinjection 直接面向原文判定）
	res := e.Inspect(&Input{Query: "id=1' OR '1'='1", Body: ""})
	if res.Score != DefaultLibiSQLiScore {
		t.Fatalf("SQLi 命中分数 = %d, 期望 %d (hits=%+v)", res.Score, DefaultLibiSQLiScore, res.Hits)
	}
	if len(res.RuleIDs) != 1 || res.RuleIDs[0] != RuleIDLibiSQLi {
		t.Fatalf("规则 ID = %v, 期望 [%s]", res.RuleIDs, RuleIDLibiSQLi)
	}
	if len(res.Categories) != 1 || res.Categories[0] != CategorySQLi {
		t.Fatalf("分类 = %v, 期望 [%s]", res.Categories, CategorySQLi)
	}
}

// TestLibiInspectXSS 验证对典型 XSS payload 的识别。
func TestLibiInspectXSS(t *testing.T) {
	e := NewLibiEngine(true, 0, 0)
	res := e.Inspect(&Input{Query: "q=<script>alert(1)</script>"})
	if res.Score != DefaultLibiXSSScore {
		t.Fatalf("XSS 命中分数 = %d, 期望 %d", res.Score, DefaultLibiXSSScore)
	}
	if len(res.Categories) != 1 || res.Categories[0] != CategoryXSS {
		t.Fatalf("分类 = %v, 期望 [%s]", res.Categories, CategoryXSS)
	}
}

// TestLibiBenign 验证正常输入不命中（防止语义通道把普通查询打成攻击）。
func TestLibiBenign(t *testing.T) {
	e := NewLibiEngine(true, 0, 0)
	for _, q := range []string{"page=2&sort=title", "keyword=golang+tutorial", "name=zhangsan"} {
		if res := e.Inspect(&Input{Query: q}); res.Score != 0 {
			t.Fatalf("正常输入 %q 被误报: score=%d hits=%v", q, res.Score, res.Hits)
		}
	}
}

// TestLibiDisabled 验证关闭状态下不产生任何命中。
func TestLibiDisabled(t *testing.T) {
	e := NewLibiEngine(false, 100, 100)
	res := e.Inspect(&Input{Query: "id=1' OR '1'='1"})
	if res.Score != 0 || len(res.Hits) != 0 {
		t.Fatalf("禁用状态不应命中: score=%d hits=%d", res.Score, len(res.Hits))
	}
}
