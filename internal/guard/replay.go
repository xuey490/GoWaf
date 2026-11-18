package guard

import "sync"

// ReplayGuard 请求防重放：记录窗口内已见过的 nonce，重复出现判定为重放。
//
// 语义：客户端在请求头携带一次性随机值（nonce），首个携带该 nonce 的请求放行，
// 窗口内再次携带同一 nonce 的请求判定为重放。窗口取 nonce 的合理生命周期
// （默认 300 秒）：窗口过后允许过期 nonce 复用位置回收，重放窗口与
// HTTP 幂等性无关联——写接口的调用方应按业务语义使用。
//
// 内存模型：key 即 nonce 本身（调用方拼接签名前缀），窗口过期惰性淘汰 +
// 周期性 Cleanup，容量无上限但按「窗口 × 请求速率」有界；nonce 由签名密钥
// HMAC 校验防伪造（见 edge 层），非客户端可控明文。
type ReplayGuard struct {
	mu    sync.Mutex
	seen  map[string]int64 // nonce -> 登记时刻（毫秒）
	ttlMS int64            // 重放判定窗口（毫秒）
}

// NewReplayGuard 创建防重放守卫。
// 参数 windowSec：重放判定窗口（秒），<=0 取默认 300。
// 返回 *ReplayGuard：就绪的守卫。
func NewReplayGuard(windowSec int) *ReplayGuard {
	if windowSec <= 0 {
		windowSec = 300
	}
	return &ReplayGuard{
		seen:  make(map[string]int64),
		ttlMS: int64(windowSec) * 1000,
	}
}

// Check 检查 nonce 是否为窗口内的首次出现。
// 参数 nonce：客户端携带的随机值（调用方应先校验签名，这里只查重）；
// 参数 nowMS：当前毫秒时间戳。
// 返回 bool：首次出现（放行并登记）返回 true；窗口内重复返回 false。
// 过期条目在查询路径惰性淘汰。
func (g *ReplayGuard) Check(nonce string, nowMS int64) bool {
	if nonce == "" {
		return true // 无 nonce 的请求不在防重放语义内，交由调用方决定
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if exp, ok := g.seen[nonce]; ok {
		if nowMS-exp <= g.ttlMS {
			return false // 窗口内重现：重放
		}
		// 过期条目：登记时刻刷新，视为新一轮
		g.seen[nonce] = nowMS
		return true
	}
	g.seen[nonce] = nowMS
	return true
}

// Cleanup 清理过期条目，防止 map 无限增长。由后台维护循环周期调用。
// 参数 nowMS：当前毫秒时间戳。
// 返回 int：本次清理的条目数。
func (g *ReplayGuard) Cleanup(nowMS int64) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	removed := 0
	for k, exp := range g.seen {
		if nowMS-exp > g.ttlMS {
			delete(g.seen, k)
			removed++
		}
	}
	return removed
}

// Size 返回当前登记的 nonce 数（监控用）。
func (g *ReplayGuard) Size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.seen)
}
