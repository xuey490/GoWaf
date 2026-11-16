package edge

import (
	"testing"

	"gowaf/internal/detect"
)

// TestShouldRecordWithAttribution 是「仅记录动作在事件页看不到」缺陷的回归用例。
//
// 背景：Bot / 精准访问控制选「仅记录」时不会改变处置动作，请求最终仍是 pass，
// 于是被 storage.sample_rate（生产环境 0.01）采样掉 99%，观察期等于没有数据。
// 修复后：只要命中过规则/策略/Bot（带 RuleIDs），就必须全量落库。
func TestShouldRecordWithAttribution(t *testing.T) {
	cases := []struct {
		name   string
		sample bool
		result *detect.Result
		want   bool
	}{
		{"采样命中，无结果", true, nil, true},
		{"采样命中，有结果", true, &detect.Result{}, true},
		{"采样未中，无结果", false, nil, false},
		{"采样未中，结果为空归因", false, &detect.Result{}, false},
		{"采样未中，Bot 归因", false, &detect.Result{RuleIDs: []string{"bot-malicious"}}, true},
		{"采样未中，策略归因", false, &detect.Result{RuleIDs: []string{"policy-12"}}, true},
		{"采样未中，规则归因", false, &detect.Result{RuleIDs: []string{"crs-913100"}}, true},
	}
	for _, c := range cases {
		if got := shouldRecord(c.sample, c.result); got != c.want {
			t.Errorf("%s: 期望 %v，实际 %v", c.name, c.want, got)
		}
	}
}
