package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDeliverWebhooksGeneric 验证通用渠道的 JSON 载荷结构与发送记录。
func TestDeliverWebhooksGeneric(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cfg := Config{Webhook: WebhookConfig{Enabled: true, GenericURL: srv.URL}}
	n := New(func() Config { return cfg }, nil)
	results := n.deliverWebhooks(cfg, []Event{{
		IP: "1.2.3.4", Site: "demo", Source: "auto",
		Reason: "测试原因", Method: "GET", Path: "/x", At: time.Now(),
	}})
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("通用渠道应发送成功: %+v", results)
	}
	if gotBody["source"] != "gowaf" {
		t.Fatalf("载荷缺少 source 字段: %v", gotBody)
	}
	if _, ok := gotBody["text"].(string); !ok || !strings.Contains(gotBody["text"].(string), "1.2.3.4") {
		t.Fatalf("载荷 text 应包含 IP: %v", gotBody["text"])
	}
	if v, ok := gotBody["count"].(float64); !ok || int(v) != 1 {
		t.Fatalf("载荷 count 应为事件数: %v", gotBody["count"])
	}
}

// TestDeliverWebhooksDisabled 验证渠道关闭时不发送。
func TestDeliverWebhooksDisabled(t *testing.T) {
	cfg := Config{Webhook: WebhookConfig{Enabled: false, GenericURL: "http://127.0.0.1:1/hook"}}
	n := New(func() Config { return cfg }, nil)
	if results := n.deliverWebhooks(cfg, []Event{{IP: "1.1.1.1", At: time.Now()}}); results != nil {
		t.Fatalf("渠道关闭应返回 nil, 实际 %+v", results)
	}
}

// TestDeliverWebhooksFailureIsolated 验证目标不可达时只记失败不 panic。
func TestDeliverWebhooksFailureIsolated(t *testing.T) {
	cfg := Config{Webhook: WebhookConfig{Enabled: true, GenericURL: "http://127.0.0.1:1/hook"}}
	n := New(func() Config { return cfg }, nil)
	results := n.deliverWebhooks(cfg, []Event{{IP: "1.1.1.1", At: time.Now()}})
	if len(results) != 1 || results[0].OK {
		t.Fatalf("不可达目标应记为失败: %+v", results)
	}
	if results[0].Error == "" {
		t.Fatalf("失败应携带原因")
	}
}

// TestFeishuSign 验证飞书签名可复现（官方示例算法）。
func TestFeishuSign(t *testing.T) {
	// 已知向量：timestamp=1591234800, secret= testimony
	got := feishuSign("testimony", 1591234800)
	if got == "" {
		t.Fatalf("签名不应为空")
	}
	// 同输入两次计算必须一致
	if got != feishuSign("testimony", 1591234800) {
		t.Fatalf("签名不具备确定性")
	}
}

// TestDingSign 验证钉钉加签参数包含 timestamp 与 sign。
func TestDingSign(t *testing.T) {
	v := dingSign("secret-demo", 1591234800000)
	if v.Get("timestamp") == "" || v.Get("sign") == "" {
		t.Fatalf("加签参数不完整: %v", v)
	}
}

// TestWebhookText 验证摘要文本包含 IP 与原因、重复 IP 去重。
func TestWebhookText(t *testing.T) {
	now := time.Now()
	text := webhookText([]Event{
		{IP: "2.2.2.2", Reason: "爆破", At: now},
		{IP: "1.1.1.1", Reason: "扫描", At: now},
		{IP: "1.1.1.1", Reason: "再次扫描", At: now},
	})
	if !strings.Contains(text, "1.1.1.1") || !strings.Contains(text, "2.2.2.2") {
		t.Fatalf("摘要缺少 IP: %s", text)
	}
	if !strings.Contains(text, "扫描") || !strings.Contains(text, "爆破") {
		t.Fatalf("摘要缺少原因: %s", text)
	}
	// 1.1.1.1 只出现一次（按 IP 去重）
	if strings.Count(text, "1.1.1.1") != 1 {
		t.Fatalf("重复 IP 应去重: %s", text)
	}
}

// TestSendTestWebhookUnconfigured 验证未配置任何地址时返回 nil。
func TestSendTestWebhookUnconfigured(t *testing.T) {
	cfg := Config{Webhook: WebhookConfig{Enabled: true}}
	if results := SendTestWebhook(cfg); results != nil {
		t.Fatalf("无目标应返回 nil: %+v", results)
	}
}
