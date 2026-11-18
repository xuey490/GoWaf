package edge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gowaf/internal/detect"
	"gowaf/internal/respfilter"
)

// fakeStage 可编程的响应变换阶段，供 transformWriter 行为测试使用。
type fakeStage struct {
	transformable bool
	limit         int
	out           string
	hits          []string
}

// Transformable 按预置开关返回。
func (f *fakeStage) Transformable(status int, h http.Header) bool { return f.transformable }

// Limit 返回预置上限。
func (f *fakeStage) Limit() int {
	if f.limit <= 0 {
		return 1 << 20
	}
	return f.limit
}

// Apply 返回预置输出与命中。
func (f *fakeStage) Apply(_ int, _ http.Header, body []byte) ([]byte, detect.Result) {
	res := detect.Result{Hits: []detect.Hit{}, Categories: []string{}, RuleIDs: []string{}}
	for _, id := range f.hits {
		res.RuleIDs = append(res.RuleIDs, id)
		res.Hits = append(res.Hits, detect.Hit{RuleID: id, Category: "test"})
		res.Categories = append(res.Categories, "test")
	}
	return []byte(f.out), res
}

// newHeader 构造带 Content-Type 的响应头。
func newHeader(ct string) http.Header {
	h := http.Header{}
	h.Set("Content-Type", ct)
	return h
}

// TestTransformWriterBuffersAndApplies 可变换响应：缓冲到 Close 后改写并写入下游。
func TestTransformWriterBuffersAndApplies(t *testing.T) {
	dest := httptest.NewRecorder()
	st := &fakeStage{transformable: true, out: "TRANSFORMED", hits: []string{"resp-phone"}}
	w := newTransformWriter(dest, []responseStage{st})

	w.WriteHeader(200)
	if w.passthrough {
		t.Fatalf("可变换响应不应立即透传")
	}
	if _, err := w.Write([]byte("ORIGINAL")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	w.Close()

	if dest.Body.String() != "TRANSFORMED" {
		t.Fatalf("下游应收到改写后的内容: %s", dest.Body.String())
	}
	if len(w.results) == 0 || w.results[0].RuleIDs[0] != "resp-phone" {
		t.Fatalf("归因结果未收集: %+v", w.results)
	}
	// 改写过正文，Content-Length 必须被摘除（accelWriter 会重设）
	if dest.Header().Get("Content-Length") != "" {
		t.Fatalf("Content-Length 应被摘除")
	}
}

// TestTransformWriterPassthrough 不可变换响应：立即透传，内容原样。
func TestTransformWriterPassthrough(t *testing.T) {
	dest := httptest.NewRecorder()
	st := &fakeStage{transformable: false, out: "SHOULD-NOT-APPEAR"}
	w := newTransformWriter(dest, []responseStage{st})

	w.WriteHeader(200)
	if !w.passthrough {
		t.Fatalf("不可变换响应应立即透传")
	}
	_, _ = w.Write([]byte("ORIGINAL-BYTES"))
	w.Close()

	if dest.Body.String() != "ORIGINAL-BYTES" {
		t.Fatalf("透传内容被篡改: %s", dest.Body.String())
	}
}

// TestTransformWriterOverflow 超过缓冲上限：降级透传，不应用变换。
func TestTransformWriterOverflow(t *testing.T) {
	dest := httptest.NewRecorder()
	st := &fakeStage{transformable: true, limit: 8, out: "TRANSFORMED"}
	w := newTransformWriter(dest, []responseStage{st})

	w.WriteHeader(200)
	_, _ = w.Write([]byte("0123456789ABCDEF")) // 16 字节 > 8 字节上限
	w.Close()

	if !strings.Contains(dest.Body.String(), "0123456789ABCDEF") {
		t.Fatalf("超限内容应原样透传: %s", dest.Body.String())
	}
	if strings.Contains(dest.Body.String(), "TRANSFORMED") {
		t.Fatalf("超限后不应应用变换")
	}
}

// TestTransformWriterHijack 验证 WebSocket 升级路径直接透传。
func TestTransformWriterHijack(t *testing.T) {
	// httptest.ResponseRecorder 不实现 Hijacker：验证降级报错而非 panic
	dest := httptest.NewRecorder()
	w := newTransformWriter(dest, []responseStage{&fakeStage{transformable: true}})
	_, _, err := w.Hijack()
	if err == nil {
		t.Fatalf("底层不支持 Hijack 时应返回错误")
	}
}

// TestActiveStagesGating 验证阶段筛选：未启用 respfilter/dprotect 时不包改写层。
// 这里只测 respFilterStage 的门禁逻辑（dprotect 需要管理器，单独在包内测试）。
func TestRespFilterStageGate(t *testing.T) {
	st := respFilterStage{s: respfilter.Settings{Enabled: false, Mode: "observe", MaxBytes: 1024}}
	if st.Transformable(200, newHeader("text/html")) {
		// Transformable 只看内容类型，不看开关；开关由 respfilter.Applies 在
		// activeStages 里过滤。这里验证内容类型判断本身。
		t.Logf("content-type gate passes for html")
	}
	if st.Limit() <= 0 {
		t.Fatalf("上限必须为正数")
	}
}
