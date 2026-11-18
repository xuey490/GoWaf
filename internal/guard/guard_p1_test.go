package guard

import (
	"sync"
	"testing"
	"time"
)

// TestReplayGuardFirstPass 同一 nonce 首次放行、窗口内重复拒绝、过期后放行。
func TestReplayGuardFirstPass(t *testing.T) {
	g := NewReplayGuard(300)
	now := int64(1000000)
	if !g.Check("n1", now) {
		t.Fatalf("首次出现应放行")
	}
	if g.Check("n1", now+1000) {
		t.Fatalf("窗口内重复应拒绝")
	}
	if !g.Check("n1", now+301*1000) {
		t.Fatalf("过期后应放行")
	}
	// 空 nonce 不在防重放语义内
	if !g.Check("", now) {
		t.Fatalf("空 nonce 应放行（语义外交由调用方处理）")
	}
}

// TestReplayGuardCleanup 过期条目可被清理。
func TestReplayGuardCleanup(t *testing.T) {
	g := NewReplayGuard(60)
	now := int64(2000000)
	g.Check("a", now)
	g.Check("b", now+1)
	if n := g.Cleanup(now + 61*1000); n != 2 {
		t.Fatalf("应清理 2 条, 实际 %d", n)
	}
	if g.Size() != 0 {
		t.Fatalf("清理后应为空")
	}
}

// TestReplayGuardConcurrent 并发 Check 安全（go test -race）。
func TestReplayGuardConcurrent(t *testing.T) {
	g := NewReplayGuard(60)
	now := int64(100)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g.Check("key", now+int64(i))
		}(i)
	}
	wg.Wait()
}

// TestWaitRoomAcquireRelease 并发阈值内的请求立即放行，归还后可再次获得。
func TestWaitRoomAcquireRelease(t *testing.T) {
	w := NewWaitRoom(2)
	if !w.Acquire(0) || !w.Acquire(0) {
		t.Fatalf("两个空槽应立即获得")
	}
	if w.Acquire(0) {
		t.Fatalf("第三个请求应立即失败（timeout=0 不排队）")
	}
	w.Release()
	if !w.Acquire(10 * time.Millisecond) {
		t.Fatalf("归还后应能获得")
	}
}

// TestWaitRoomQueue 等候室排队：被占满时阻塞等候，归还后等到。
func TestWaitRoomQueue(t *testing.T) {
	w := NewWaitRoom(1)
	w.Acquire(0)
	go func() {
		time.Sleep(50 * time.Millisecond)
		w.Release()
	}()
	if !w.Acquire(2 * time.Second) {
		t.Fatalf("等候 50ms 归还后应获得槽位")
	}
}

// TestWaitRoomTimeout 等候超时返回失败。
func TestWaitRoomTimeout(t *testing.T) {
	w := NewWaitRoom(1)
	w.Acquire(0)
	if w.Acquire(50 * time.Millisecond) {
		t.Fatalf("槽位被占时应超时失败")
	}
	a, i := w.Stats()
	if a != 1 || i != 0 {
		t.Fatalf("占用统计异常: active=%d idle=%d", a, i)
	}
}
