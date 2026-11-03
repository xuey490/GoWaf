package guard

import "testing"

func TestHitTrackerReachesThreshold(t *testing.T) {
	tr := NewHitTracker()
	base := int64(1_700_000_000_000)
	const window = int64(600 * 1000)

	for i := 1; i <= 4; i++ {
		n, reached := tr.Add("1.2.3.4", base+int64(i)*1000, window, 5)
		if reached {
			t.Fatalf("第 %d 次不该触发，实际触发", i)
		}
		if n != i {
			t.Fatalf("第 %d 次计数 = %d", i, n)
		}
	}
	_, reached := tr.Add("1.2.3.4", base+5000, window, 5)
	if !reached {
		t.Fatal("第 5 次应触发封禁")
	}
	// 触发后计数归零，下一个窗口重新累计
	if got := tr.WindowHits("1.2.3.4"); got != 0 {
		t.Fatalf("触发后计数应归零，实际 %d", got)
	}
}

func TestHitTrackerWindowExpires(t *testing.T) {
	tr := NewHitTracker()
	base := int64(1_700_000_000_000)
	const window = int64(600 * 1000)

	for i := 1; i <= 4; i++ {
		tr.Add("1.2.3.4", base+int64(i)*1000, window, 5)
	}
	// 隔了 11 分钟再打，窗口应重置，不应直接触发
	n, reached := tr.Add("1.2.3.4", base+11*60*1000, window, 5)
	if reached {
		t.Fatal("跨窗口不应累计到阈值")
	}
	if n != 1 {
		t.Fatalf("新窗口计数应为 1，实际 %d", n)
	}
}

func TestHitTrackerThresholdZeroNeverBans(t *testing.T) {
	tr := NewHitTracker()
	for i := 0; i < 100; i++ {
		if _, reached := tr.Add("9.9.9.9", int64(i)*1000, 60000, 0); reached {
			t.Fatal("threshold=0 表示关闭，不应触发")
		}
	}
}

func TestHitTrackerResetClearsWindow(t *testing.T) {
	tr := NewHitTracker()
	base := int64(1_700_000_000_000)
	for i := 0; i < 3; i++ {
		tr.Add("5.6.7.8", base+int64(i)*1000, 600000, 5)
	}
	tr.Reset("5.6.7.8")
	if got := tr.WindowHits("5.6.7.8"); got != 0 {
		t.Fatalf("Reset 后应为 0，实际 %d", got)
	}
}
