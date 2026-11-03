package edge

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"gowaf/internal/proxy"
)

// routeWithBodyLimit 构造一个指定 body 上限的 Route。
func routeWithBodyLimit(n int) *proxy.Route {
	return &proxy.Route{MaxBodyBytes: n}
}

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

func TestReadBody_GET_NoBody(t *testing.T) {
	h := &Handler{}
	r := httptest.NewRequest("GET", "/x", nil)
	body, skip, tooLarge := h.readBody(nil, r, routeWithBodyLimit(1024))
	if body != nil || skip || tooLarge {
		t.Fatal("GET 请求不应触发任何 body 逻辑")
	}
}
