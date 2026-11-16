package edge

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gowaf/internal/cache"
)

// newTestCache 构造一个启用状态、带 .js 后缀规则的空缓存管理器。
func newTestCache(t *testing.T) *cache.Manager {
	t.Helper()
	m := cache.NewManager()
	st := cache.Settings{
		Enabled:          true,
		MaxBytes:         8 << 20,
		MaxObjectBytes:   4 << 20,
		DefaultTTL:       time.Minute,
		Compress:         true,
		Brotli:           true,
		CompressMinBytes: 1024,
	}
	defs := []cache.RuleDef{{
		ID: 1, Name: "js", Kind: cache.KindSuffix, Pattern: ".js",
		TTL: time.Minute, Priority: 100, Enabled: true,
	}}
	if err := m.Reload(defs, st); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	return m
}

// TestAccelWriterCacheWithPerWriteFlush 复现 FlushInterval=-1 的真实链路：
// 反向代理每写一块都会调用 Flush，缓存仍必须能写入并在下一次命中。
func TestAccelWriterCacheWithPerWriteFlush(t *testing.T) {
	m := newTestCache(t)
	rule, ok := m.Match("site", "/app.js", true)
	if !ok {
		t.Fatal("规则未命中 /app.js")
	}
	key := cache.Key("site", "example.com", "/app.js", "")
	body := bytes.Repeat([]byte("console.log(1);"), 200)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	aw := newAccelWriter(rec, req, m, m.Settings(), &rule, key)
	aw.Header().Set("Content-Type", "application/javascript")
	aw.WriteHeader(http.StatusOK)
	// 模拟 ReverseProxy：每块写完立即 Flush
	for i := 0; i < len(body); i += 512 {
		end := i + 512
		if end > len(body) {
			end = len(body)
		}
		if _, err := aw.Write(body[i:end]); err != nil {
			t.Fatalf("Write: %v", err)
		}
		aw.Flush()
	}
	aw.Close()

	e, hit := m.Get(key, time.Now())
	if !hit {
		t.Fatal("每块 Flush 之后缓存未写入：Flush 仍然把缓冲打穿了")
	}
	if !bytes.Equal(e.Body, body) {
		t.Fatalf("缓存正文被改写，期望 %d 字节得到 %d 字节", len(body), len(e.Body))
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("首回响应状态 %d", rec.Code)
	}
}

// TestAccelWriterDecodesUpstreamGzip 上游已 gzip 时，缓存必须存原始字节，
// 并按客户端 Accept-Encoding 重新协商编码。
func TestAccelWriterDecodesUpstreamGzip(t *testing.T) {
	m := newTestCache(t)
	rule, ok := m.Match("site", "/big.js", true)
	if !ok {
		t.Fatal("规则未命中 /big.js")
	}
	key := cache.Key("site", "example.com", "/big.js", "")

	plain := bytes.Repeat([]byte("function f(){return 42};\n"), 120)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(plain); err != nil {
		t.Fatalf("gzip 写入: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip 关闭: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/big.js", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")

	aw := newAccelWriter(rec, req, m, m.Settings(), &rule, key)
	aw.Header().Set("Content-Type", "application/javascript")
	aw.Header().Set("Content-Encoding", "gzip")
	aw.Header().Set("ETag", `"abc-gzip"`)
	aw.Header().Set("Content-Length", strconv.Itoa(len(gz.Bytes())))
	aw.WriteHeader(http.StatusOK)
	if _, err := aw.Write(gz.Bytes()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	aw.Close()

	e, hit := m.Get(key, time.Now())
	if !hit {
		t.Fatal("上游 gzip 响应未入缓存")
	}
	if !bytes.Equal(e.Body, plain) {
		t.Fatalf("缓存应存解码后的原始字节：期望 %d 字节，得到 %d 字节", len(plain), len(e.Body))
	}
	if strings.EqualFold(e.Header.Get("ETag"), `"abc-gzip"`) {
		t.Fatal("缓存头里应剔除上游 -gzip 的 ETag")
	}

	// 客户端拿到的是重新压缩后的编码，且可解回原文
	enc := rec.Header().Get("Content-Encoding")
	if enc != "br" && enc != "gzip" {
		t.Fatalf("期望重新压缩为 br/gzip，实际 %q", enc)
	}
	var r io.Reader = bytes.NewReader(rec.Body.Bytes())
	if enc == "gzip" {
		zr, err := gzip.NewReader(r)
		if err != nil {
			t.Fatalf("响应 gzip 解压失败: %v", err)
		}
		r = zr
	}
	out, err := io.ReadAll(r)
	if err != nil && enc == "gzip" {
		t.Fatalf("读取响应体: %v", err)
	}
	if enc == "gzip" && !bytes.Equal(out, plain) {
		t.Fatalf("响应正文与原文不一致：期望 %d 字节，得到 %d 字节", len(plain), len(out))
	}
	if rec.Header().Get("ETag") != "" {
		t.Fatal("响应头应剔除上游 ETag")
	}
}
