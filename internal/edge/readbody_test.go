package edge

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"gowaf/internal/proxy"
)

// routeWithBodyLimit 构造一个指定 body 上限的 Route。
// 参数 n：body 检测上限（字节）。
// 返回 *proxy.Route：只设置 MaxBodyBytes 的最小路由对象。
func routeWithBodyLimit(n int) *proxy.Route {
	return &proxy.Route{MaxBodyBytes: n}
}

// TestReadBody_OversizeKnownLength_PassesThrough 验证 ContentLength 已知且超限的场景：
// 请求体一个字节都不读、标记 skipBody 原样透传，体积上限交由上游裁决。
// 关键断言：tooLarge=false（不拒绝）、body=nil、透传后 r.Body 仍完整可读。
func TestReadBody_OversizeKnownLength_PassesThrough(t *testing.T) {
	// ContentLength 已知且超限：不读不关 body，跳过检测透传（大文件上传场景）
	h := &Handler{}
	big := bytes.Repeat([]byte("x"), 300*1024) // 300KB > 256KB
	r := httptest.NewRequest("POST", "/api/platform/video/", bytes.NewReader(big))
	r.ContentLength = int64(len(big))

	body, skip, tooLarge := h.readBody(nil, r, routeWithBodyLimit(256*1024))
	if tooLarge || body != nil {
		t.Fatalf("超限请求应透传而非拒绝: tooLarge=%v body=%d", tooLarge, len(body))
	}
	if !skip {
		t.Fatal("应标记 skipBody=true")
	}
	// body 必须原封不动可读（未被消费），透传后才不会坏
	var got bytes.Buffer
	if _, err := got.ReadFrom(r.Body); err != nil || got.Len() != len(big) {
		t.Fatalf("透传后 body 应完整可读: n=%d err=%v", got.Len(), err)
	}
}

// TestReadBody_Chunked_SkipsDetection 验证 ContentLength 未知（chunked）的场景：
// 不读 body、跳过检测、透传——读满才知道超限时 body 已被消费，无法回退。
func TestReadBody_Chunked_SkipsDetection(t *testing.T) {
	// ContentLength 未知（chunked）：不读、跳过检测、透传
	h := &Handler{}
	big := bytes.Repeat([]byte("y"), 300*1024)
	r := httptest.NewRequest("POST", "/upload", bytes.NewReader(big))
	r.ContentLength = -1

	body, skip, tooLarge := h.readBody(nil, r, routeWithBodyLimit(256*1024))
	if tooLarge || body != nil || !skip {
		t.Fatalf("chunked 超限应透传: tooLarge=%v skip=%v", tooLarge, skip)
	}
}

// TestReadBody_UnderLimit_Buffered 验证上限内的正常小 body：内容被完整读出，
// 且回填进 r.Body 供规则检测与转发阶段重读。
func TestReadBody_UnderLimit_Buffered(t *testing.T) {
	// 正常小 body：读入缓冲并回填，供规则检测复用
	h := &Handler{}
	payload := []byte(`{"q":"1' or 1=1"}`)
	r := httptest.NewRequest("POST", "/api/x", bytes.NewReader(payload))

	body, skip, tooLarge := h.readBody(nil, r, routeWithBodyLimit(256*1024))
	if tooLarge || skip {
		t.Fatalf("小 body 不应跳过或拒绝")
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("body 内容不符")
	}
	// 回填后的 body 应可重读（转发用）
	var got bytes.Buffer
	_, _ = got.ReadFrom(r.Body)
	if !bytes.Equal(got.Bytes(), payload) {
		t.Fatal("回填后 body 应可重读")
	}
}

// TestReadBody_LyingContentLength_Rejects 验证 ContentLength 谎报的场景：
// 声明小、实际大，读到 max+1 才发现、body 已被消费，只能拒绝（tooLarge=true）。
func TestReadBody_LyingContentLength_Rejects(t *testing.T) {
	// ContentLength 谎报（声明小于上限、实际超限）：body 已被消费，只能拒绝
	h := &Handler{}
	big := bytes.Repeat([]byte("z"), 300*1024)
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(big))
	r.ContentLength = 100 // 谎报

	_, _, tooLarge := h.readBody(nil, r, routeWithBodyLimit(256*1024))
	if !tooLarge {
		t.Fatal("谎报 ContentLength 的超限 body 应拒绝")
	}
}

// TestReadBody_GET_NoBody 验证 GET 请求不触发任何 body 逻辑：
// body、skipBody、tooLarge 三个返回值均为零值。
func TestReadBody_GET_NoBody(t *testing.T) {
	h := &Handler{}
	r := httptest.NewRequest("GET", "/x", nil)
	body, skip, tooLarge := h.readBody(nil, r, routeWithBodyLimit(1024))
	if body != nil || skip || tooLarge {
		t.Fatal("GET 请求不应触发任何 body 逻辑")
	}
}
