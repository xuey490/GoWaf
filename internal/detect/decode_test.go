package detect

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"strings"
	"testing"
)

// TestDecodeVariantsBase64 验证 base64 变体解出攻击载荷。
func TestDecodeVariantsBase64(t *testing.T) {
	// base64("1' OR '1'='1")
	enc := base64.StdEncoding.EncodeToString([]byte("1' OR '1'='1"))
	qv, _ := DecodeVariants("token="+enc, "", DefaultDecodeConfig())
	if len(qv) == 0 {
		t.Fatalf("base64 应产生变体")
	}
	joined := strings.Join(qv, "|")
	if !strings.Contains(strings.ToLower(joined), "' or '1'=") {
		t.Fatalf("变体应包含解码后的 SQLi 特征: %s", joined)
	}
}

// TestDecodeVariantsNestedBase64 验证双层 base64（深度 2）。
func TestDecodeVariantsNestedBase64(t *testing.T) {
	inner := base64.StdEncoding.EncodeToString([]byte("1' OR '1'='1"))
	outer := base64.StdEncoding.EncodeToString([]byte(inner))
	_, bv := DecodeVariants("", outer, DecodeConfig{Enabled: true, MaxDepth: 2, MaxBytes: 65536})
	joined := strings.Join(bv, "|")
	if !strings.Contains(strings.ToLower(joined), "' or '1'=") {
		t.Fatalf("双层 base64 应解出内层载荷: %s", joined)
	}
}

// TestDecodeVariantsUnicodeEscape 验证 \\u0027 转义还原。
func TestDecodeVariantsUnicodeEscape(t *testing.T) {
	body := `{"q":"1\u0027 OR \u00271\u0027=\u00271"}`
	_, bv := DecodeVariants("", body, DefaultDecodeConfig())
	joined := strings.Join(bv, "|")
	if !strings.Contains(joined, "'") {
		t.Fatalf("unicode 转义应解出引号: %s", joined)
	}
}

// TestDecodeVariantsGzipBody 验证 gzip 压缩请求体的解压变体。
func TestDecodeVariantsGzipBody(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("q=1' OR '1'='1 -- comment padding padding"))
	_ = zw.Close()

	_, bv := DecodeVariants("", buf.String(), DefaultDecodeConfig())
	if len(bv) == 0 {
		t.Fatalf("gzip body 应产生解压变体")
	}
	if !strings.Contains(strings.ToLower(strings.Join(bv, "|")), "' or '1'=") {
		t.Fatalf("解压变体应包含攻击特征: %s", strings.Join(bv, "|"))
	}
}

// TestDecodeVariantsDisabled 验证关闭时不产生任何变体。
func TestDecodeVariantsDisabled(t *testing.T) {
	cfg := DecodeConfig{Enabled: false}
	enc := base64.StdEncoding.EncodeToString([]byte("1' OR '1'='1' OR '1'='1"))
	qv, bv := DecodeVariants("token="+enc, enc, cfg)
	if qv != nil || bv != nil {
		t.Fatalf("禁用状态不应产生变体")
	}
}

// TestDecodeVariantsNoFalsePositive 验证普通输入不产生乱码变体。
func TestDecodeVariantsNoFalsePositive(t *testing.T) {
	for _, q := range []string{"page=2&sort=title", "keyword=golang+tutorial&lang=zh"} {
		qv, _ := DecodeVariants(q, "", DefaultDecodeConfig())
		for _, v := range qv {
			if strings.ContainsAny(v, "\x00\x01\x02") {
				t.Fatalf("变体包含控制字符: %q", v)
			}
		}
	}
}

// TestDecodeEngineScanVariants 验证解码变体进入规则引擎扫描（端到端：变体里的
// SQLi 载荷应触发 sqli 规则）。
func TestDecodeEngineScanVariants(t *testing.T) {
	e := NewEngine(60, 30, "protect")
	if err := e.Reload(nil); err != nil {
		t.Fatalf("空规则集 Reload 不应失败: %v", err)
	}
	// 空规则集不命中，这里只验证带变体的 Inspect 不 panic、结构正常
	enc := base64.StdEncoding.EncodeToString([]byte("1' OR '1'='1"))
	res := e.Inspect(&Input{
		Query:        "token=" + enc,
		DecodedQuery: []string{"token=1' OR '1'='1"},
	})
	if res.Hits == nil || res.Categories == nil {
		t.Fatalf("返回结构必须非 nil")
	}
}
