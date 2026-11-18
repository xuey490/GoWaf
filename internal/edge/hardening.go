package edge

// 检测引擎加固：超时熔断 + panic 隔离 + 失效策略（fail-open / fail-closed）。
//
// 背景：检测与控制面同进程（见仓库架构决策），自研引擎、CRS/Coraza、libinjection
// 任何一个 panic 都会带倒整个数据面；一条病态正则或规则包把单请求检测拖到几十秒，
// 连接池就会被慢慢占满。本文件给三个引擎调用套上同一层保护：
//
//	超时：单引擎超过 engine.timeout_ms 毫秒，视为该引擎本次失效；
//	panic：recover 住并降级，进程不倒；
//	失效策略：engine.fail_mode = open（放行并记录，默认）/ closed（拦截）。
//
// 已知取舍：超时后引擎协程仍在后台跑（RE2/Coraza 自身有 regex 超时，通常很快
// 自然结束），结果写进带缓冲 channel 后被 GC，不回收协程本身——换来的是主请求
// 路径零阻塞。这是有意的工程折衷，不要改成 context 取消（Coraza 不支持）。

import (
	"log"
	"runtime/debug"
	"time"

	"gowaf/internal/detect"
	"gowaf/internal/metrics"
)

// engineErrorRuleID 引擎降级时写入事件的规则标识：事件页按 ID 精确检索降级请求。
const engineErrorRuleID = "engine-degraded"

// engineBudget 返回单引擎的检测超时预算。
// 参数 ms：设置项 engine.timeout_ms 的当前值。
// 返回 time.Duration：<=0 或非法时回退 2 秒；下限 50ms 防止误配成 0 后每次都超时。
func engineBudget(ms int) time.Duration {
	if ms <= 0 {
		ms = 2000
	}
	if ms < 50 {
		ms = 50
	}
	return time.Duration(ms) * time.Millisecond
}

// inspectGuarded 在超时与 panic 保护下执行一次引擎检测。
// 参数 name：引擎名（用于日志、指标与事件归因）；budget：超时预算；
// 参数 fn：引擎检测闭包，如 func() detect.Result { return engine.Inspect(&in) }。
// 返回 (detect.Result, bool)：第一值为检测结果（降级时是合成的占位结果）；
// 第二值为是否发生降级（超时或 panic），调用方据此执行失效策略。
func inspectGuarded(name string, budget time.Duration, fn func() detect.Result) (detect.Result, bool) {
	// 结果携带降级标记一起过通道：panic 产生的占位结果与正常结果必须可区分，
	// 否则 select 侧无法得知引擎是否异常退出。
	outcome := make(chan inspectOutcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[edge] 检测引擎 %s panic（已隔离）: %v\n%s", name, r, debug.Stack())
				outcome <- inspectOutcome{degradedResult(name, name+" 检测器异常退出（panic 已隔离）"), true}
			}
		}()
		outcome <- inspectOutcome{fn(), false}
	}()

	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case o := <-outcome:
		return o.res, o.degraded
	case <-timer.C:
		log.Printf("[edge] 检测引擎 %s 超时（>%s），本次请求按失效策略处理", name, budget)
		metrics.DetectErrors.WithLabelValues(name, "timeout").Inc()
		return degradedResult(name, name+" 检测超时（"+budget.String()+"）"), true
	}
}

// inspectOutcome 引擎检测结果与降级标记的组合（走 channel 传递）。
type inspectOutcome struct {
	res      detect.Result // 检测结果；降级时为合成的占位结果
	degraded bool          // 是否发生了超时或 panic
}

// degradedResult 构造引擎降级时的占位结果。
// 分数为 0：失效策略在 handleEngineFailure 里统一决定最终动作，这里只负责
// 让降级事件带归因（RuleIDs 非空 → shouldRecord 必记），不污染评分。
// 参数 name：引擎名；参数 reason：降级原因（写入命中明细的 Name 字段）。
// 返回 detect.Result：带 engine-degraded 命中记录的空结果。
func degradedResult(name, reason string) detect.Result {
	return detect.Result{
		Hits: []detect.Hit{{
			RuleID:   engineErrorRuleID,
			Name:     name + ": " + reason,
			Category: "engine",
			Target:   "engine",
			Score:    0,
		}},
		Categories: []string{"engine"},
		RuleIDs:    []string{engineErrorRuleID},
	}
}

// handleEngineFailure 执行引擎失效策略。必须在 applyGuardAction 之后调用：
// applyGuardAction 会按分数重判动作，而降级结果的分数是 0，先判会被覆盖成 pass。
// 参数 result：本次请求的最终检测结果（就地修改）；参数 failMode：open / closed；
// 参数 observe：引擎当前是否观察模式（观察模式只记录，closed 也不得真拦）。
// 返回值无。
func handleEngineFailure(result *detect.Result, failMode string, observe bool) {
	if result == nil || result.Action == "ban" {
		// 高危规则命中即封的结论不因引擎降级推翻：规则引擎已经给出了确凿证据，
		// CRS/libi 超时与否不影响它。
		return
	}
	if failMode == "closed" && !observe {
		// fail-closed：宁可错杀不可放过。直接置 block，不参与评分重判。
		result.Action = "block"
		return
	}
	// fail-open（默认）与观察模式：放行。动作归零为 pass，让降级事件以
	// 「有归因的放行」落库，管理员能在事件页搜 engine-degraded 看到降级频率，
	// 再决定是否收紧策略。
	result.Action = "pass"
	result.Score = 0
	result.Ban = false
}
