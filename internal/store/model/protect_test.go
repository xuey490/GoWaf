package model

import "testing"

// TestParseSiteProtect 验证 ParseSiteProtect 对 protect_cfg 各种输入的解析与校验行为。
// 覆盖：空串与 "null" 视为跟随全局（返回 nil）、非法 JSON 报错、enabled/mode 取值校验、
// blockScore 与 challengeScore 的范围及大小关系约束、负数参数报错，
// 以及合法配置的完整解析与仅覆盖部分维度（其余字段保持零值=跟随全局）。
func TestParseSiteProtect(t *testing.T) {
	cases := []struct {
		name    string       // 子测试名称
		raw     string       // protect_cfg 原始输入
		wantNil bool         // 期望返回 nil（跟随全局）
		wantErr bool         // 期望返回错误
		want    *SiteProtect // 期望解析出的配置（结构体整体相等比较）
	}{
		{name: "空串=跟随全局", raw: "", wantNil: true},
		{name: "null=跟随全局", raw: "null", wantNil: true},
		{name: "非法JSON", raw: "{bad", wantErr: true},
		{name: "enabled非法", raw: `{"enabled":2}`, wantErr: true},
		{name: "mode非法", raw: `{"enabled":1,"mode":"off"}`, wantErr: true},
		{name: "block<=challenge", raw: `{"enabled":1,"blockScore":30,"challengeScore":30}`, wantErr: true},
		{name: "block超范围", raw: `{"enabled":1,"blockScore":2000}`, wantErr: true},
		{name: "负数", raw: `{"enabled":1,"ipRps":-1}`, wantErr: true},
		{
			name: "合法覆盖",
			raw:  `{"enabled":1,"mode":"protect","blockScore":50,"challengeScore":20,"ipRps":100,"ipBurst":200,"ipPerMinute":2000}`,
			want: &SiteProtect{Enabled: 1, Mode: "protect", BlockScore: 50, ChallengeScore: 20, IPRPS: 100, IPBurst: 200, IPPerMinute: 2000},
		},
		{
			name: "仅开限流覆盖",
			raw:  `{"enabled":1,"ipRps":10}`,
			want: &SiteProtect{Enabled: 1, IPRPS: 10},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParseSiteProtect(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错，得到 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			if tc.wantNil {
				if p != nil {
					t.Fatalf("期望 nil，得到 %+v", p)
				}
				return
			}
			if *p != *tc.want {
				t.Fatalf("得到 %+v，期望 %+v", *p, *tc.want)
			}
		})
	}
}
