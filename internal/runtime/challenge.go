package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strings"

	"gowaf/internal/guard"
)

// challengeSecret 读取人机校验通行证的 HMAC 密钥。
//
// 直接用 cfg.Admin.JWTSecretFile 当密钥是错的：那是文件路径字符串，
// 相当于把一个公开常量当密钥用，知道默认路径（data/jwt.key）的人就能自己
// 签出通行证绕过校验。这里改成读文件内容（与 admin/auth 使用同一份密钥材料）。
// 文件缺失或过短时退化为进程内随机密钥：通行证在重启后失效，但不可伪造。
// 参数 path：JWT 密钥文件路径。
// 返回 []byte：至少 32 字节的签名密钥。
func challengeSecret(path string) []byte {
	if path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			if d, derr := hex.DecodeString(strings.TrimSpace(string(raw))); derr == nil && len(d) >= 32 {
				return d
			}
			if len(raw) >= 32 {
				return raw
			}
		} else {
			log.Printf("[runtime] 读取挑战签名密钥失败: %v（改用进程内随机密钥）", err)
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// 极端情况下随机源不可用：用固定串兜底，至少保证进程能起来
		return []byte("gowaf-challenge-fallback-secret-32b")
	}
	return buf
}

// newChallengeLimiter 构建人机校验的软阈值计数器。
//
// 与硬限流器分开实例：软限流只负责"超过这个速度就先验一下人"，不应该消耗硬限流的令牌，
// 否则用户刚验完、正常翻两页就被自己的软计数卡住。
// 参数 rps：每秒阈值（<=0 表示关闭，构造时给一个不可能触发的大值）；
// 参数 perMinute：每分钟阈值（<=0 表示不启用分钟维度）。
// 返回 *guard.Limiter：就绪的限流器实例。
func newChallengeLimiter(rps, perMinute int) *guard.Limiter {
	if rps <= 0 {
		// 关闭状态：给一个不可能达到的速率，避免 NewLimiter 的默认值把功能悄悄打开
		rps = 1 << 20
	}
	burst := rps * 4
	if burst < 20 {
		burst = 20
	}
	return guard.NewLimiter(rps, burst, perMinute)
}

// ChallengeLimiter 返回当前人机校验软阈值计数器（原子读取，Reload 时整体替换）。
// 返回 *guard.Limiter：尚未初始化或已禁用时返回 nil，调用方需判空。
func (r *Runtime) ChallengeLimiter() *guard.Limiter { return r.challLimiter.Load() }
