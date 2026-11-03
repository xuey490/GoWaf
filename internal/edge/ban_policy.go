package edge

import "fmt"

// banPolicy 是自动封禁的完整策略参数，全部来自控制台可热改的设置项。
type banPolicy struct {
	windowSec  int // 统计窗口（秒）
	threshold  int // 窗口内命中多少次触发封禁
	ttlSec     int // 每次封禁时长（秒）
	maxTTLSec  int // 封禁时长上限（秒），0 = 不限制
	maxStrikes int // 连续第几次触发升级为永久，0 = 永不升级
}

// decide 根据"这是第几次触发"算出封禁到期时间与原因。
//
// 返回 expires = 0 表示永久封禁；reason 会原样写进 bans 表，面板上直接可见，
// 所以文案必须让人一眼看懂"为什么封、封多久、还有几次就永久"。
func (p banPolicy) decide(strikes int, nowMS int64) (expires int64, reason string, permanent bool) {
	if strikes < 1 {
		strikes = 1
	}

	if p.maxStrikes > 0 && strikes >= p.maxStrikes {
		return 0, fmt.Sprintf("自动永久封禁：%d 秒内命中规则 %d 次，已连续触发 %d 次（阈值 %d）",
			p.windowSec, p.threshold, strikes, p.maxStrikes), true
	}

	ttl := int64(p.ttlSec) * 1000
	if ttl <= 0 {
		// 0 会造成"封了但立刻过期"，等于没封；给个兜底下限
		ttl = 60 * 1000
	}
	if p.maxTTLSec > 0 {
		if limit := int64(p.maxTTLSec) * 1000; ttl > limit {
			ttl = limit
		}
	}

	if p.maxStrikes > 0 {
		return nowMS + ttl,
			fmt.Sprintf("自动封禁：%d 秒内命中规则 %d 次（第 %d/%d 次，再犯即永久）",
				p.windowSec, p.threshold, strikes, p.maxStrikes), false
	}
	return nowMS + ttl,
		fmt.Sprintf("自动封禁：%d 秒内命中规则 %d 次", p.windowSec, p.threshold), false
}
