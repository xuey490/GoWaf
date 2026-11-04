package notify

import (
	"strings"
	"testing"
	"time"

	"gowaf/internal/store/model"
)

// TestRenderDigest 摘要邮件要包含所有封禁 IP 与中文原因标签。
func TestRenderDigest(t *testing.T) {
	cfg := Config{Subject: "【GoWAF 告警】"}
	evs := []Event{
		{IP: "1.2.3.4", Site: "a.com", Country: "美国", Source: "auto", Reason: "600 秒内命中规则 5 次", Score: 60, At: time.Now()},
		{IP: "5.6.7.8", Site: "b.com", Source: "intel", Reason: "来源 IP 命中威胁情报（全局情报）", At: time.Now()},
	}
	subject, body := RenderDigest(cfg, evs)
	if !strings.Contains(subject, "2 个 IP") {
		t.Fatalf("邮件主题应包含 IP 数量，实际: %s", subject)
	}
	for _, want := range []string{"1.2.3.4", "5.6.7.8", "a.com", "威胁情报", "自动封禁"} {
		if !strings.Contains(body, want) {
			t.Fatalf("正文缺少 %q", want)
		}
	}
	if strings.Contains(body, "<script") {
		t.Fatal("正文不应出现未转义的脚本标签")
	}
}

// TestRenderDigestEscapes 事件字段来自请求（路径/UA），必须转义防注入。
func TestRenderDigestEscapes(t *testing.T) {
	_, body := RenderDigest(Config{}, []Event{{
		IP: "1.2.3.4", Path: `/<script>alert(1)</script>`, Reason: "x", At: time.Now(),
	}})
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatal("危险字符未转义")
	}
}

// TestNotifierAggregates 一个窗口内的多条事件应合并成一次发送，
// 同一 IP 在冷却期内不重复通知。
func TestNotifierAggregates(t *testing.T) {
	cfg := Config{
		Enabled: true, Host: "127.0.0.1", Port: 1, To: []string{"ops@example.com"},
		Window: 0, Cooldown: time.Hour,
	}
	var logs []model.NotifyLog
	n := New(func() Config { return cfg }, func(l model.NotifyLog) { logs = append(logs, l) })

	now := time.Now()
	n.Send(Event{IP: "1.1.1.1", At: now})
	n.Send(Event{IP: "2.2.2.2", At: now})
	n.Send(Event{IP: "1.1.1.1", At: now}) // 冷却期内，应被丢弃
	n.tick(now.Add(time.Second))

	if len(logs) != 1 {
		t.Fatalf("期望只发送 1 次，实际 %d 次", len(logs))
	}
	if logs[0].Count != 2 {
		t.Fatalf("期望聚合 2 条事件，实际 %d", logs[0].Count)
	}
	// 端口 1 必然连不上，借此验证失败也会落记录且不 panic
	if logs[0].Status != "fail" {
		t.Fatalf("连不上 SMTP 时应记 fail，实际 %s", logs[0].Status)
	}
	if !strings.Contains(logs[0].Target, "ops@example.com") {
		t.Fatalf("记录里应有收件人，实际 %s", logs[0].Target)
	}
}

// TestNotifierDisabled 关闭状态不入队、不发送。
func TestNotifierDisabled(t *testing.T) {
	var logs []model.NotifyLog
	n := New(func() Config { return Config{Enabled: false} }, func(l model.NotifyLog) { logs = append(logs, l) })
	n.Send(Event{IP: "1.1.1.1", At: time.Now()})
	n.tick(time.Now().Add(time.Minute))
	if len(logs) != 0 {
		t.Fatal("关闭时不应发送任何通知")
	}
}
