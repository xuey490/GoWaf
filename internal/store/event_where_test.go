package store

import (
	"strings"
	"testing"
)

// TestBuildEventWhereKeywordCoversRuleIDs 是「事件搜不到策略/Bot 拦截」缺陷的回归用例。
//
// 背景：精准访问控制命中的规则 ID 是 policy-<id>，Bot 命中是 bot-<category>，
// 两者只存在于 rule_ids / categories 字段。旧实现的关键词只覆盖 path/ua/query，
// 因此在事件页搜「policy」「bot-malicious」「策略名」全部无结果。
func TestBuildEventWhereKeywordCoversRuleIDs(t *testing.T) {
	where, args := buildEventWhere(EventQuery{Keyword: "policy-12"})
	for _, field := range []string{"path", "ua", "query", "rule_ids", "categories", "client_ip"} {
		if !strings.Contains(where, field+" LIKE ?") {
			t.Fatalf("关键词未覆盖字段 %s，实际子句：%s", field, where)
		}
	}
	if len(args) != 6 {
		t.Fatalf("应绑定 6 个关键词参数，实际 %d：%v", len(args), args)
	}
	for _, a := range args {
		if a != "%policy-12%" {
			t.Fatalf("参数应统一为 %%%s%%，实际 %v", "policy-12", a)
		}
	}
}

// TestBuildEventWhereCategoryUsesQuotedJSON 验证分类过滤按 JSON 元素精确匹配：
// 库里 categories 是 ["bot"] / ["policy"] 这样的 JSON 文本，若用裸 LIKE '%bot%'
// 会把 "botnet" 之类的分类也捞进来。
func TestBuildEventWhereCategoryUsesQuotedJSON(t *testing.T) {
	where, args := buildEventWhere(EventQuery{Category: "policy"})
	if !strings.Contains(where, `categories LIKE ?`) {
		t.Fatalf("分类过滤应使用 LIKE，实际：%s", where)
	}
	if len(args) != 1 || args[0] != `%"policy"%` {
		t.Fatalf("分类参数应带引号，实际：%v", args)
	}
}

// TestBuildEventWhereCombinesFilters 验证多条件按 AND 拼接且参数顺序稳定。
func TestBuildEventWhereCombinesFilters(t *testing.T) {
	where, args := buildEventWhere(EventQuery{
		Site: "a.example", Action: "block", Category: "bot",
		IP: "1.2.3.4", Keyword: "sqlmap", From: 100, To: 200,
	})
	if !strings.HasPrefix(where, " WHERE ") {
		t.Fatalf("应以 WHERE 开头，实际：%s", where)
	}
	if got := strings.Count(where, " AND "); got != 6 {
		t.Fatalf("7 个条件之间应有 6 个 AND，实际 %d：%s", got, where)
	}
	want := []any{"a.example", "block", `%"bot"%`, "1.2.3.4"}
	for i, w := range want {
		if args[i] != w {
			t.Fatalf("第 %d 个参数应为 %v，实际 %v（全部：%v）", i, w, args[i], args)
		}
	}
}

// TestBuildEventWhereEmpty 验证零值查询不产生 WHERE 子句，避免全表扫描被误伤。
func TestBuildEventWhereEmpty(t *testing.T) {
	where, args := buildEventWhere(EventQuery{})
	if where != "" || len(args) != 0 {
		t.Fatalf("空查询应返回空子句，实际 where=%q args=%v", where, args)
	}
}
