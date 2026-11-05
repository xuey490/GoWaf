package edge

import (
	"strings"
	"testing"
)

// defaultPolicy 返回测试用的默认策略：10 分钟窗口、5 次命中、封 1 小时、
// 单次封禁上限 24 小时、连续 3 次永久。
// 返回 banPolicy：与线上常见配置对应的策略对象。
func defaultPolicy() banPolicy {
	return banPolicy{windowSec: 600, threshold: 5, ttlSec: 3600, maxTTLSec: 86400, maxStrikes: 3}
}

// TestBanPolicyTemporary 验证前两次触发只做限时封禁：到期时间为 now+ttl，
// 不升级永久，且文案带「第 x/3 次」的升级提示。
func TestBanPolicyTemporary(t *testing.T) {
	p := defaultPolicy()
	now := int64(1_700_000_000_000)

	for _, strikes := range []int{1, 2} {
		expires, reason, permanent := p.decide(strikes, now)
		if permanent {
			t.Fatalf("第 %d 次不应该永久", strikes)
		}
		if want := now + 3600*1000; expires != want {
			t.Fatalf("第 %d 次到期时间 = %d，期望 %d", strikes, expires, want)
		}
		if !strings.Contains(reason, "第 "+itoa(strikes)+"/3 次") {
			t.Fatalf("文案应提示还有几次永久，实际：%s", reason)
		}
	}
}

// TestBanPolicyPermanentOnThirdStrike 验证第 3 次触发（达到 maxStrikes）升级为永久：
// expires 必须为 0（永久封禁），文案写明「永久」。
func TestBanPolicyPermanentOnThirdStrike(t *testing.T) {
	p := defaultPolicy()
	expires, reason, permanent := p.decide(3, 1_700_000_000_000)
	if !permanent {
		t.Fatal("第 3 次应该永久")
	}
	if expires != 0 {
		t.Fatalf("永久封禁 expires 必须是 0，实际 %d", expires)
	}
	if !strings.Contains(reason, "永久") {
		t.Fatalf("文案应写明永久，实际：%s", reason)
	}
}

// TestBanPolicyNoPermanentWhenDisabled 验证 maxStrikes=0（关闭连击升级）时，
// 无论累计触发多少次都不会永久封禁，始终按 ttl 限时封禁。
func TestBanPolicyNoPermanentWhenDisabled(t *testing.T) {
	p := defaultPolicy()
	p.maxStrikes = 0
	expires, _, permanent := p.decide(99, 1000)
	if permanent {
		t.Fatal("maxStrikes=0 时不应升级为永久")
	}
	if expires != 1000+3600*1000 {
		t.Fatalf("到期时间不对：%d", expires)
	}
}

// TestBanPolicyTTLCapped 验证 ttlSec 超过 maxTTLSec 时被封禁时长上限截断：
// 想封 48 小时的请求被压到 24 小时。
func TestBanPolicyTTLCapped(t *testing.T) {
	p := defaultPolicy()
	p.ttlSec = 48 * 3600 // 想封 48 小时
	p.maxStrikes = 0
	expires, _, _ := p.decide(1, 0)
	if want := int64(24 * 3600 * 1000); expires != want {
		t.Fatalf("应被 ban_max_ttl 截到 24h，实际 %d，期望 %d", expires, want)
	}
}

// TestBanPolicyZeroTTLFallsBack 验证 ttlSec=0 时使用 60 秒兜底时长，
// 避免「封了但立刻过期」等于没封。
func TestBanPolicyZeroTTLFallsBack(t *testing.T) {
	p := defaultPolicy()
	p.ttlSec = 0
	p.maxStrikes = 0
	expires, _, _ := p.decide(1, 0)
	if expires <= 0 {
		t.Fatalf("ttl=0 会造成秒解封，应给兜底时长，实际 %d", expires)
	}
}

// itoa 测试辅助：把非负整数转为十进制字符串，供断言拼接「第 x/3 次」文案。
// 参数 n：非负整数。
// 返回 string：十进制表示；负数行为未定义（测试中不会出现）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
