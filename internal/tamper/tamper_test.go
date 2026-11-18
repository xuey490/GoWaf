package tamper

import (
	"strings"
	"testing"
)

// TestStoreAndCompare 验证基线写入与比对的主流程。
func TestStoreAndCompare(t *testing.T) {
	m := New()
	body := []byte("<html>trusted content</html>")
	if err := m.Store("site1", "/index.html", body, "text/html"); err != nil {
		t.Fatalf("写入基线失败: %v", err)
	}
	b, ok := m.BaselineFor("site1", "/index.html")
	if !ok {
		t.Fatalf("基线应存在")
	}
	if !m.Matches(b, body) {
		t.Fatalf("相同内容应比对一致")
	}
	tampered := []byte("<html>trusted content<script src=//evil></script></html>")
	if m.Matches(b, tampered) {
		t.Fatalf("篡改内容不应比对一致")
	}
	// block 模式的替换副本
	if copyBody := m.Copy(b); copyBody == nil || string(copyBody) != string(body) {
		t.Fatalf("基线副本应可取出且与原内容一致")
	}
}

// TestStoreEmptyRejected 验证空内容被拒绝。
func TestStoreEmptyRejected(t *testing.T) {
	m := New()
	if err := m.Store("s", "/a", nil, "text/html"); err == nil {
		t.Fatalf("空内容不应写入基线")
	}
}

// TestAppliesExactPath 验证受保护路径的精确匹配语义。
func TestAppliesExactPath(t *testing.T) {
	s := Settings{Enabled: true, Paths: []string{"/index.html", "/logo.png"}, MaxBytes: 1024}
	if !Applies(&s, "GET", "/index.html") {
		t.Fatalf("精确路径应命中")
	}
	if Applies(&s, "GET", "/index.html.bak") {
		t.Fatalf("非精确匹配不应命中（防篡改不做前缀，避免把整站缓存都关了）")
	}
	if Applies(&s, "POST", "/index.html") {
		t.Fatalf("写请求不参与")
	}
	if Applies(&Settings{Enabled: false, Paths: s.Paths}, "GET", "/index.html") {
		t.Fatalf("未启用不应命中")
	}
}

// TestTransformable 验证响应类型门禁。
func TestTransformable(t *testing.T) {
	if !Transformable(200, "text/html; charset=utf-8") {
		t.Fatalf("html 应在比对范围")
	}
	if !Transformable(200, "image/png") {
		t.Fatalf("图片应在比对范围")
	}
	if Transformable(302, "text/html") {
		t.Fatalf("302 不比对")
	}
	if Transformable(200, "application/octet-stream") {
		t.Fatalf("二进制流不比对")
	}
}

// TestListAndCount 验证基线列表与计数。
func TestListAndCount(t *testing.T) {
	m := New()
	_ = m.Store("b", "/b.html", []byte("b"), "text/html")
	_ = m.Store("a", "/a.html", []byte("a"), "text/html")
	if m.Count() != 2 {
		t.Fatalf("计数 = %d", m.Count())
	}
	list := m.List()
	if len(list) != 2 || list[0].Site != "a" || list[1].Site != "b" {
		t.Fatalf("列表应按站点排序: %+v", list)
	}
	if !strings.HasPrefix(list[0].SHA256, "") || len(list[0].SHA256) != 64 {
		t.Fatalf("哈希应为 64 位 hex: %s", list[0].SHA256)
	}
	m.Clear()
	if m.Count() != 0 {
		t.Fatalf("清空后计数应为 0")
	}
}
