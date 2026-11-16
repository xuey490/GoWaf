package edge

// 本文件把 Bot / 精准访问控制引擎的判定结果转换成统一的 detect.Result，
// 让事件表、指标、阈值重判与封禁逻辑复用同一条通路。

import (
	"strconv"

	"gowaf/internal/bot"
	"gowaf/internal/detect"
	"gowaf/internal/policy"
)

// botResult 把 Bot 判定转换成检测结果。
// 参数 v：Bot 引擎的判定；返回 detect.Result：分数、命中分类与规则 ID。
// 只有 block/ban 携带分数：log / challenge 已单独处置过，不能因合并分数被再次拦截。
func botResult(v bot.Verdict) detect.Result {
	score := 0
	switch v.Action {
	case bot.ActionBlock:
		score = 60
	case bot.ActionBan:
		score = 100
	}
	category := v.Category
	if category == "" {
		category = "bot"
	}
	action := string(v.Action)
	if action == "" {
		action = "log"
	}
	return detect.Result{
		Score:      score,
		Action:     action,
		Hits:       []detect.Hit{{RuleID: "bot-" + category, Name: v.Reason, Category: "bot", Score: score, Target: "ua"}},
		Categories: []string{"bot"},
		RuleIDs:    []string{"bot-" + category},
	}
}

// policyResult 把精准访问控制命中转换成检测结果。
// 参数 m：策略命中；返回 detect.Result：动作与展示字段。
// 与 Bot 同理，只有 block/ban 带分数，避免 log/challenge 被阈值二次升级为拦截。
func policyResult(m *policy.Match) detect.Result {
	if m == nil {
		return detect.Result{}
	}
	score := 0
	switch m.Action {
	case policy.ActionBlock:
		score = 60
	case policy.ActionBan:
		score = 100
	}
	action := string(m.Action)
	if action == "" {
		action = "log"
	}
	id := "policy-" + strconv.FormatUint(uint64(m.PolicyID), 10)
	return detect.Result{
		Score:      score,
		Action:     action,
		Hits:       []detect.Hit{{RuleID: id, Name: m.Reason, Category: "policy", Score: score, Target: "request"}},
		Categories: []string{"policy"},
		RuleIDs:    []string{id},
	}
}
