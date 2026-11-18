package edge

import (
	"strings"
	"testing"
	"time"

	"gowaf/internal/detect"
)

// TestInspectGuardedPanic 验证引擎 panic 被隔离为降级结果而不是打倒进程。
func TestInspectGuardedPanic(t *testing.T) {
	res, degraded := inspectGuarded("test", 500*time.Millisecond, func() detect.Result {
		panic("boom")
	})
	if !degraded {
		t.Fatalf("panic 应标记为降级")
	}
	if !strings.Contains(res.Hits[0].RuleID, "engine-degraded") {
		t.Fatalf("降级结果应带 engine-degraded 归因: %v", res.Hits)
	}
}

// TestInspectGuardedTimeout 验证超预算的引擎调用被熔断。
func TestInspectGuardedTimeout(t *testing.T) {
	res, degraded := inspectGuarded("test", 30*time.Millisecond, func() detect.Result {
		time.Sleep(300 * time.Millisecond)
		return detect.Result{Score: 999, Action: "block"}
	})
	if !degraded {
		t.Fatalf("超时应标记为降级")
	}
	if res.Score != 0 {
		t.Fatalf("降级结果不应携带原分数: %d", res.Score)
	}
}

// TestInspectGuardedNormal 验证正常路径原样返回、不降级。
func TestInspectGuardedNormal(t *testing.T) {
	res, degraded := inspectGuarded("test", time.Second, func() detect.Result {
		return detect.Result{Score: 42, Action: "block"}
	})
	if degraded {
		t.Fatalf("正常执行不应降级")
	}
	if res.Score != 42 || res.Action != "block" {
		t.Fatalf("结果被篡改: %+v", res)
	}
}

// TestHandleEngineFailure 验证 open/closed 两种失效策略与观察模式语义。
func TestHandleEngineFailure(t *testing.T) {
	// fail-open：放行但保留归因
	res := degradedResult("rules", "超时")
	handleEngineFailure(&res, "open", false)
	if res.Action != "pass" || res.Score != 0 {
		t.Fatalf("fail-open 应放行: action=%s score=%d", res.Action, res.Score)
	}
	if len(res.RuleIDs) == 0 {
		t.Fatalf("降级归因不应被清除")
	}

	// fail-closed：拦截
	res2 := degradedResult("rules", "超时")
	handleEngineFailure(&res2, "closed", false)
	if res2.Action != "block" {
		t.Fatalf("fail-closed 应拦截: %s", res2.Action)
	}

	// 观察模式：即使 closed 也只记录，与「观察模式不拦截」的全局语义一致
	res3 := degradedResult("rules", "超时")
	handleEngineFailure(&res3, "closed", true)
	if res3.Action != "pass" {
		t.Fatalf("观察模式下 closed 也不应拦截: %s", res3.Action)
	}

	// ban 结论不因降级推翻
	res4 := degradedResult("rules", "超时")
	res4.Action = "ban"
	handleEngineFailure(&res4, "open", false)
	if res4.Action != "ban" {
		t.Fatalf("显式 ban 不应被降级策略覆盖")
	}
}

// TestEngineBudget 验证超时预算的兜底与下限。
func TestEngineBudget(t *testing.T) {
	if engineBudget(0) != 2*time.Second {
		t.Fatalf("缺省应回退 2 秒")
	}
	if engineBudget(-5) != 2*time.Second {
		t.Fatalf("负值应回退 2 秒")
	}
	if engineBudget(10) != 50*time.Millisecond {
		t.Fatalf("过小值应抬升到 50ms 下限")
	}
	if engineBudget(500) != 500*time.Millisecond {
		t.Fatalf("合法值应原样返回")
	}
}
