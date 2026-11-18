package guard

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// envB64 把环境信号编码成挑战页 JS 同款格式：base64(JSON)。
func envB64(t *testing.T, e map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func validEnv() map[string]any {
	wd := false
	hc := 8
	pl := 4
	pt := 450.0
	return map[string]any{
		"wd": wd, "hc": hc, "sc": "1920x1080", "tz": "Asia/Shanghai",
		"wgl": "Google Inc. (NVIDIA)/ANGLE (NVIDIA, GeForce RTX)", "ln": "zh-CN",
		"pl": pl, "pt": pt,
	}
}

func TestCheckEnvSignalValid(t *testing.T) {
	if !CheckEnvSignal(envB64(t, validEnv())) {
		t.Fatal("完整合法的环境信号应通过")
	}
}

func TestCheckEnvSignalRejects(t *testing.T) {
	cases := []struct {
		name string
		mut  func(map[string]any)
	}{
		{"webdriver=true", func(e map[string]any) { e["wd"] = true }},
		{"缺 webdriver", func(e map[string]any) { delete(e, "wd") }},
		{"缺核数", func(e map[string]any) { delete(e, "hc") }},
		{"核数为 0（jsdom 特征）", func(e map[string]any) { e["hc"] = 0 }},
		{"屏幕缺失", func(e map[string]any) { delete(e, "sc") }},
		{"屏幕非法", func(e map[string]any) { e["sc"] = "0x0" }},
		{"屏幕非分辨率串", func(e map[string]any) { e["sc"] = "big" }},
		{"时区缺失", func(e map[string]any) { delete(e, "tz") }},
		{"WebGL 缺失", func(e map[string]any) { delete(e, "wgl") }},
		{"页面耗时为负", func(e map[string]any) { e["pt"] = -5.0 }},
		{"页面耗时离谱", func(e map[string]any) { e["pt"] = 99999999.0 }},
	}
	for _, tc := range cases {
		e := validEnv()
		tc.mut(e)
		if CheckEnvSignal(envB64(t, e)) {
			t.Errorf("%s: 应拒绝", tc.name)
		}
	}
	// 非法 base64 / 空
	for _, bad := range []string{"", "not-base64!!", "e30="} { // e30= = "{}"
		if CheckEnvSignal(bad) {
			t.Errorf("%q 应拒绝", bad)
		}
	}
}

func TestPowIssuedAt(t *testing.T) {
	now := time.Now()
	tok := NewChallenger(make([]byte, 32), 30*time.Minute).ChallengeToken("1.2.3.4", now, 16)
	at, ok := PowIssuedAt(tok)
	if !ok {
		t.Fatal("合法令牌应可解析签发时间")
	}
	diff := at - now.Add(-time.Millisecond).UnixMilli() // 允许 1ms 舍入
	if diff < -1 || diff > 5 {
		t.Fatalf("签发时间偏差过大: %d ms", diff)
	}
	if _, ok := PowIssuedAt("garbage"); ok {
		t.Fatal("非法令牌应解析失败")
	}
	if _, ok := PowIssuedAt(""); ok {
		t.Fatal("空令牌应解析失败")
	}
}
