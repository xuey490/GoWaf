package respfilter

import (
	"net/http"
	"strings"
	"testing"
)

// htmlHeader 构造 text/html 响应头。
func htmlHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "text/html; charset=utf-8")
	return h
}

// containsString 判断列表是否包含字符串（测试辅助）。
func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// TestApplyMaskPhone 验证 observe→mask 模式下手机号被打码且保留前后缀。
func TestApplyMaskPhone(t *testing.T) {
	s := DefaultSettings()
	s.Enabled = true
	s.Mode = "mask"
	body := []byte(`<p>联系电话 13812345678，备用 15987654321。</p>`)
	out, res := Apply(&s, 200, htmlHeader(), body)
	text := string(out)
	if strings.Contains(text, "13812345678") {
		t.Fatalf("手机号未被脱敏: %s", text)
	}
	if !strings.Contains(text, "138****5678") || !strings.Contains(text, "159****4321") {
		t.Fatalf("脱敏格式不符合预期: %s", text)
	}
	if !containsString(res.RuleIDs, "resp-phone") {
		t.Fatalf("缺少 resp-phone 归因: %v", res.RuleIDs)
	}
	// 边界：13 位数字不应被截出 11 位
	out2, _ := Apply(&s, 200, htmlHeader(), []byte(`订单号 1138123456780`))
	if strings.Contains(string(out2), "****") {
		t.Fatalf("长数字串被误判为手机号: %s", out2)
	}
}

// TestApplyObserveKeepsBody 验证观察模式只归因不改写。
func TestApplyObserveKeepsBody(t *testing.T) {
	s := DefaultSettings()
	s.Enabled = true
	s.Mode = "observe"
	original := `user: 13812345678`
	out, res := Apply(&s, 200, htmlHeader(), []byte(original))
	if string(out) != original {
		t.Fatalf("观察模式不应改写响应: %s", out)
	}
	if len(res.RuleIDs) == 0 {
		t.Fatalf("观察模式仍应产生归因")
	}
}

// TestApplyCredentials 验证 API 密钥与 JWT 被替换为占位符。
func TestApplyCredentials(t *testing.T) {
	s := DefaultSettings()
	s.Enabled = true
	s.Mode = "mask"
	body := []byte(`{"access":"AKIAIOSFODNN7EXAMPLE","jwt":"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJVadQssw5c"}`)
	out, _ := Apply(&s, 200, htmlHeader(), body)
	text := string(out)
	if strings.Contains(text, "AKIAIOSFODNN7EXAMPLE") || strings.Contains(text, "eyJhbGciOiJIUzI1NiJ9") {
		t.Fatalf("凭据未被脱敏: %s", text)
	}
	if !strings.Contains(text, "[REDACTED]") || !strings.Contains(text, "[REDACTED-JWT]") {
		t.Fatalf("占位符不符合预期: %s", text)
	}
}

// TestTransformable 验证内容类型与状态码门禁。
func TestTransformable(t *testing.T) {
	jsonHeader := http.Header{}
	jsonHeader.Set("Content-Type", "application/json")
	if !Transformable(200, jsonHeader) {
		t.Fatalf("JSON 2xx 应可扫描")
	}
	if Transformable(302, htmlHeader()) {
		t.Fatalf("302 不应扫描")
	}
	if Transformable(500, htmlHeader()) {
		t.Fatalf("5xx 不应扫描")
	}
	binHeader := http.Header{}
	binHeader.Set("Content-Type", "image/png")
	if Transformable(200, binHeader) {
		t.Fatalf("二进制类型不应扫描")
	}
	sseHeader := http.Header{}
	sseHeader.Set("Content-Type", "text/event-stream")
	if Transformable(200, sseHeader) {
		t.Fatalf("SSE 不应扫描")
	}
}

// TestAppliesExcludePaths 验证排除前缀不参与扫描。
func TestAppliesExcludePaths(t *testing.T) {
	s := DefaultSettings()
	s.Enabled = true
	s.ExcludePaths = []string{"/api/export", "/internal"}
	if Applies(&s, "/api/export/data") {
		t.Fatalf("排除前缀不应扫描")
	}
	if !Applies(&s, "/dashboard") {
		t.Fatalf("普通路径应扫描")
	}
	disabled := DefaultSettings()
	if Applies(&disabled, "/dashboard") {
		t.Fatalf("未启用时不应扫描")
	}
}

// TestApplyEmailDisabled 验证 email 类别默认不在启用集。
func TestApplyEmailDisabled(t *testing.T) {
	s := DefaultSettings()
	s.Enabled = true
	s.Mode = "mask"
	out, res := Apply(&s, 200, htmlHeader(), []byte(`联系 admin@example.com`))
	if strings.Contains(string(out), "***") {
		t.Fatalf("email 默认不应启用: %s", out)
	}
	if containsString(res.RuleIDs, "resp-email") {
		t.Fatalf("email 默认不应产生归因")
	}
}
