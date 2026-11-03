package edge

import (
	"strings"
	"testing"
)

// 默认策略：10 分钟窗口、5 次命中、封 1 小时、连 3 次永久。
func defaultPolicy() banPolicy {
	return banPolicy{windowSec: 600, threshold: 5, ttlSec: 3600, maxTTLSec: 86400, maxStrikes: 3}
}

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

func TestBanPolicyTTLCapped(t *testing.T) {
	p := defaultPolicy()
	p.ttlSec = 48 * 3600 // 想封 48 小时
	p.maxStrikes = 0
	expires, _, _ := p.decide(1, 0)
	if want := int64(24 * 3600 * 1000); expires != want {
		t.Fatalf("应被 ban_max_ttl 截到 24h，实际 %d，期望 %d", expires, want)
	}
}

func TestBanPolicyZeroTTLFallsBack(t *testing.T) {
	p := defaultPolicy()
	p.ttlSec = 0
	p.maxStrikes = 0
	expires, _, _ := p.decide(1, 0)
	if expires <= 0 {
		t.Fatalf("ttl=0 会造成秒解封，应给兜底时长，实际 %d", expires)
	}
}

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
