package logship

import (
	"strings"
	"testing"
	"time"

	"gowaf/internal/store/model"
)

// TestRFC5424Format 校验帧结构与 PRI 计算。
func TestRFC5424Format(t *testing.T) {
	e := model.Event{
		TS: 1, Site: "demo", ClientIP: "1.2.3.4", Method: "GET", Host: "a.com",
		Path: "/x?q", Status: 403, Action: "block", Score: 60,
		Categories: `["sqli"]`, RuleIDs: `["r1"]`, ReqID: "abc", UA: "curl", Country: "CN",
	}
	frame := rfc5424(e, time.UnixMilli(1763471941000), 4, "waf-node")
	// facility=4 severity=4 → PRI=36
	if !strings.HasPrefix(frame, "<36>1 2025-11-18T") {
		t.Fatalf("PRI 或时间戳错误: %s", frame)
	}
	if !strings.Contains(frame, " waf-node gowaf - - [") {
		t.Fatalf("hostname/SDID 结构错误: %s", frame)
	}
	if !strings.Contains(frame, `site="demo"`) || !strings.Contains(frame, `client_ip="1.2.3.4"`) {
		t.Fatalf("SD 参数缺失: %s", frame)
	}
	// RFC5424 规定 PARAM-VALUE 内的 ] 必须转义为 \]
	if !strings.Contains(frame, `categories=[\"sqli\"\]`) {
		t.Fatalf("SD 参数值错误: %s", frame)
	}
	if !strings.HasSuffix(frame, "\n") {
		t.Fatalf("帧应以换行结尾")
	}
}

// TestEscParamValue PARAM-VALUE 特殊字符转义。
func TestEscParamValue(t *testing.T) {
	if got := esc(`a"b\c]d`); got != `a\"b\\c\]d` {
		t.Fatalf("转义结果错误: %s", got)
	}
}

// TestSeverityOf 动作→severity 映射。
func TestSeverityOf(t *testing.T) {
	cases := map[string]int{"block": 4, "ban": 4, "challenge": 5, "log": 6, "pass": 6}
	for a, want := range cases {
		if got := severityOf(a); got != want {
			t.Fatalf("severity(%s) = %d, 期望 %d", a, got, want)
		}
	}
}

// TestPassesMinAction 动作级别过滤。
func TestPassesMinAction(t *testing.T) {
	cases := []struct {
		min, action string
		want        bool
	}{
		{"all", "pass", true},
		{"log", "pass", false},
		{"log", "block", true},
		{"block", "block", true},
		{"block", "log", false},
		{"", "pass", true},
	}
	for _, c := range cases {
		if got := passesMinAction(c.min, c.action); got != c.want {
			t.Fatalf("passesMinAction(%q,%q) = %v, 期望 %v", c.min, c.action, got, c.want)
		}
	}
}

// TestSendDisabled 未启用时 Send 是空操作。
func TestSendDisabled(t *testing.T) {
	s := New(Config{}, 8)
	defer s.Close()
	s.Send(model.Event{Action: "block"})
	if s.Dropped() != 0 || s.Sent() != 0 {
		t.Fatalf("未启用时不应有任何投递")
	}
}

// TestSendMinActionFilter 过滤后的事件不入队。
func TestSendMinActionFilter(t *testing.T) {
	s := New(Config{Enabled: true, MinAction: "block"}, 8)
	s.Send(model.Event{Action: "pass"})
	s.Send(model.Event{Action: "block"})
	// 只投一条（worker 可能尚未消费，直接看队列长度）
	if len(s.ch) != 1 {
		t.Fatalf("应只入队 1 条, 实际 %d", len(s.ch))
	}
	s.Close()
}
