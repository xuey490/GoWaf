package guard

import "time"

// WaitRoom 等候室：并发阈值外的请求排队等候，超出等候能力才返回「稍后重试」。
//
// 语义：站点面对突发流量（大促、放票）时，把「同时打到上游的请求数」压在
// maxActive 以内——阈值内的请求立即放行；阈值外的请求在队列中等候
// （channel 的阻塞接收天然 FIFO），最多等 timeout；等不到则请求方拿到
// 429 与 Retry-After，浏览器按 Retry-After 自动重试，等效于继续排队。
//
// 与限流（Limiter）的区别：限流按 IP 维度拒绝超额请求，等候室按站点全局
// 并发平滑放行——被拒的用户是「等一会儿再试」，而不是「被判定为攻击者」。
//
// 释放纪律：Acquire 成功的调用方必须在请求结束时调用 Release（defer），
// 否则槽位泄漏、站点整体卡死。未成功获得槽位的调用方不得调用 Release。
type WaitRoom struct {
	slots chan struct{}
}

// NewWaitRoom 创建等候室。
// 参数 maxActive：同时放行到上游的最大并发数（<=0 时按 1 处理，最保守）；
// 参数 queue：无关容量语义，仅为保留参数（队列等待由 timeout 控制）。
// 返回 *WaitRoom：就绪的等候室。
func NewWaitRoom(maxActive int) *WaitRoom {
	if maxActive <= 0 {
		maxActive = 1
	}
	return &WaitRoom{slots: make(chan struct{}, maxActive)}
}

// Acquire 尝试获得一个放行槽位。
// 参数 timeout：最长等候时长，<=0 表示立即返回（不排队）。
// 返回 bool：获得槽位返回 true（调用方必须 Release）；等候超时返回 false。
func (w *WaitRoom) Acquire(timeout time.Duration) bool {
	if timeout <= 0 {
		select {
		case w.slots <- struct{}{}:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case w.slots <- struct{}{}:
		return true
	case <-timer.C:
		return false
	}
}

// Release 归还槽位。仅在 Acquire 返回 true 后调用。
func (w *WaitRoom) Release() {
	select {
	case <-w.slots:
	default:
	}
}

// Stats 返回当前占用与空闲槽位数（监控用）。
// 返回 (int, int)：占用数、空闲数。
func (w *WaitRoom) Stats() (active, idle int) {
	n := len(w.slots)
	cap := cap(w.slots)
	return n, cap - n
}
