package store_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"gowaf/internal/detect"
	"gowaf/internal/store/model"
)

// builtinRuleForTest 与 store.builtinRule 结构一致（那边不导出，这里单独声明）。
type builtinRuleForTest struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Category string   `yaml:"category"`
	Score    int      `yaml:"score"`
	Targets  []string `yaml:"targets"`
	Keywords []string `yaml:"keywords"`
	Pattern  string   `yaml:"pattern"`
	Action   string   `yaml:"action"`
	Priority int      `yaml:"priority"`
	Remark   string   `yaml:"remark"`
	Group    string   `yaml:"group"`
}

// TestBuiltinRulesCompile 保证规则库能被解析、每条正则都能编译。
//
// 规则库是防护的命脉：一条写坏的正则会让 Compile 整体失败，运行时保留旧快照
// 等于新规则全部不生效。所以规则库必须有这条回归测试。
func TestBuiltinRulesCompile(t *testing.T) {
	raw, err := os.ReadFile("../../rules/builtin.yaml")
	if err != nil {
		t.Skip("找不到 rules/builtin.yaml，跳过")
	}
	var list []builtinRuleForTest
	if err := yaml.Unmarshal(raw, &list); err != nil {
		t.Fatalf("YAML 解析失败: %v", err)
	}
	if len(list) < 180 {
		t.Fatalf("内置规则数量偏少: %d（目标 200+）", len(list))
	}

	seen := map[string]bool{}
	rows := make([]model.Rule, 0, len(list))
	for _, r := range list {
		if r.ID == "" || r.Name == "" || r.Category == "" {
			t.Fatalf("规则缺少必填字段: %+v", r)
		}
		if seen[r.ID] {
			t.Fatalf("规则 ID 重复: %s", r.ID)
		}
		seen[r.ID] = true
		targets := strings.Join(r.Targets, ",")
		if targets == "" {
			targets = "path,query,body"
		}
		kws, _ := json.Marshal(r.Keywords)
		rows = append(rows, model.Rule{
			ID: r.ID, Name: r.Name, Category: r.Category, Score: r.Score,
			Targets: targets, Keywords: string(kws), Pattern: r.Pattern,
			Action: r.Action, Enabled: 1, Priority: r.Priority,
		})
	}

	if _, err := detect.Compile(rows); err != nil {
		t.Fatalf("规则编译失败: %v", err)
	}
}
