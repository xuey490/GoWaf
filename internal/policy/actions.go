package policy

// 精准访问控制的两个「带参数动作」：auth（页面密码/访问认证）与 rate_limit（自定义 CC 规则）。
//
// 与 allow/block 等无参动作不同，这两个动作在命中后需要额外配置才能执行：
//
//	auth：认证方式（Basic Auth 或密码页）、bcrypt 密码哈希、会话时长；
//	rate_limit：统计窗口、阈值、超限后的处置（拦截或挑战）。
//
// 参数以 JSON 存于 access_policies.config 列，Compile 时解析进不可变快照：
//   - 编译期强校验（密码哈希必须是 bcrypt、窗口/阈值在合法区间），
//     坏配置整体拒绝 Reload，旧快照保留（fail-safe 与条件树一致）；
//   - auth / rate_limit 策略的条件树不允许依赖 body（编译期拒绝）——
//     这两个动作是「门禁」，必须在读 body 之前的早期阶段（缓存查找前）执行，
//     否则命中缓存的资源会绕过门禁；也因此第 8 步完整求值会跳过它们，防止重复计数。
//
// 密码只存 bcrypt 哈希（golang.org/x/crypto/bcrypt），明文仅在控制台创建/修改
// 时经过内存，不落库、不进审计快照的 JSON（控制台回显用掩码）。

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ActionAuth 页面密码 / 访问认证动作。
const ActionAuth Action = "auth"

// ActionRateLimit 自定义 CC 规则动作。
const ActionRateLimit Action = "rate_limit"

// policyAuthCookiePrefix 认证通行证 Cookie 名前缀，完整名为 waf_auth_<policyID>。
// 按策略独立 Cookie：多个认证策略各自持票，互不踢对方的会话。
const policyAuthCookiePrefix = "waf_auth_"

// 默认值与边界。
const (
	DefaultAuthTTLSec  = 8 * 3600 // 认证通行证默认有效期：8 小时
	MaxAuthTTLSec      = 30 * 24 * 3600
	DefaultCCWindowSec = 60    // 自定义 CC 默认统计窗口：60 秒
	DefaultCCThreshold = 120   // 自定义 CC 默认阈值：窗口内 120 次
	MaxCCWindowSec     = 86400 // 窗口上限 1 天
	MaxCCThreshold     = 1000000
)

// AuthUser Basic Auth 的一个账号。
type AuthUser struct {
	User         string `json:"user"`                   // 用户名
	PasswordHash string `json:"passwordHash,omitempty"` // bcrypt 哈希（明文不落库）
}

// AuthConfig 认证门禁配置（action=auth）。
type AuthConfig struct {
	Mode         string     `json:"mode"`                   // basic：HTTP Basic 认证 / page：密码页
	Realm        string     `json:"realm,omitempty"`        // Basic 认证的 realm 提示
	Title        string     `json:"title,omitempty"`        // 密码页标题，留空用默认
	Users        []AuthUser `json:"users,omitempty"`        // Basic 模式的账号列表（≥1）
	PasswordHash string     `json:"passwordHash,omitempty"` // Page 模式的访问密码哈希
	TTLSec       int        `json:"ttlSec,omitempty"`       // Page 模式通行证有效期（秒），默认 8 小时
}

// RateLimitConfig 自定义 CC 规则配置（action=rate_limit）。
type RateLimitConfig struct {
	WindowSec int    `json:"windowSec"`          // 统计窗口（秒），10~86400
	Threshold int    `json:"threshold"`          // 窗口内允许的最大请求数（按「策略+IP」计数）
	OnExceed  string `json:"onExceed,omitempty"` // 超限处置：block 拦截（默认）/ challenge 人机校验
}

// parseAuthConfig 解析并校验 auth 动作的 config JSON。
// 参数 raw：config 列原文；返回 (*AuthConfig, error)：缺字段或格式非法即报错。
func parseAuthConfig(raw string) (*AuthConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("auth 策略缺少 config（认证方式与密码）")
	}
	var c AuthConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("auth config JSON 非法: %w", err)
	}
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode != "basic" && c.Mode != "page" {
		return nil, fmt.Errorf("auth mode 只能是 basic 或 page")
	}
	switch c.Mode {
	case "basic":
		if len(c.Users) == 0 {
			return nil, fmt.Errorf("basic 认证至少需要一个账号")
		}
		for _, u := range c.Users {
			if strings.TrimSpace(u.User) == "" {
				return nil, fmt.Errorf("basic 认证账号的用户名不能为空")
			}
			if !isBcryptHash(u.PasswordHash) {
				return nil, fmt.Errorf("账号 %s 的密码哈希必须是 bcrypt 格式", u.User)
			}
		}
		if strings.TrimSpace(c.Realm) == "" {
			c.Realm = "Restricted"
		}
	case "page":
		if !isBcryptHash(c.PasswordHash) {
			return nil, fmt.Errorf("page 认证缺少访问密码（bcrypt 哈希）")
		}
	}
	if c.TTLSec <= 0 {
		c.TTLSec = DefaultAuthTTLSec
	}
	if c.TTLSec > MaxAuthTTLSec {
		return nil, fmt.Errorf("auth 通行证有效期不能超过 %d 秒", MaxAuthTTLSec)
	}
	return &c, nil
}

// parseRateLimitConfig 解析并校验 rate_limit 动作的 config JSON。
// 参数 raw：config 列原文；返回 (*RateLimitConfig, error)：窗口/阈值越界即报错。
func parseRateLimitConfig(raw string) (*RateLimitConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("rate_limit 策略缺少 config（窗口与阈值）")
	}
	var c RateLimitConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("rate_limit config JSON 非法: %w", err)
	}
	if c.WindowSec <= 0 {
		c.WindowSec = DefaultCCWindowSec
	}
	if c.WindowSec < 10 || c.WindowSec > MaxCCWindowSec {
		return nil, fmt.Errorf("rate_limit 窗口必须是 10~%d 秒", MaxCCWindowSec)
	}
	if c.Threshold <= 0 {
		c.Threshold = DefaultCCThreshold
	}
	if c.Threshold > MaxCCThreshold {
		return nil, fmt.Errorf("rate_limit 阈值不能超过 %d", MaxCCThreshold)
	}
	c.OnExceed = strings.ToLower(strings.TrimSpace(c.OnExceed))
	if c.OnExceed == "" {
		c.OnExceed = "block"
	}
	if c.OnExceed != "block" && c.OnExceed != "challenge" {
		return nil, fmt.Errorf("rate_limit 超限动作只能是 block 或 challenge")
	}
	return &c, nil
}

// isBcryptHash 判断字符串是否形如 bcrypt 哈希（$2a$/$2b$/$2y$ 前缀，60 字符）。
// 参数 s：待判断字符串。
// 返回 bool：形似 bcrypt 哈希返回 true。
func isBcryptHash(s string) bool {
	if len(s) != 60 {
		return false
	}
	return strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$")
}

// compileActionConfig 按 action 解析动作参数，编译进不可变快照。
// 参数 action：策略动作；参数 raw：config 列原文。
// 返回 (auth, rateLimit, error)：与 action 对应的配置；无参动作返回 nil,nil,nil。
func compileActionConfig(action Action, raw string) (*AuthConfig, *RateLimitConfig, error) {
	switch action {
	case ActionAuth:
		c, err := parseAuthConfig(raw)
		return c, nil, err
	case ActionRateLimit:
		c, err := parseRateLimitConfig(raw)
		return nil, c, err
	default:
		return nil, nil, nil
	}
}

// PolicyAuthCookieName 返回策略对应的认证 Cookie 名。
// 参数 policyID：策略 ID。
// 返回 string：waf_auth_<policyID>。
func PolicyAuthCookieName(policyID uint) string {
	return policyAuthCookiePrefix + fmt.Sprint(policyID)
}

// AuthConfigByID 按策略 ID 取认证配置（用于 /.waf-auth/<id> 密码提交端点，
// 该请求可能不命中任何条件树，需按 ID 直查）。
// 参数 id：策略 ID。
// 返回 (*AuthConfig, bool)：策略存在且动作是 auth 时返回配置。
func (s *Set) AuthConfigByID(id uint) (*AuthConfig, bool) {
	if s == nil {
		return nil, false
	}
	for _, p := range s.policies {
		if p.id == id && p.action == ActionAuth {
			return p.auth, true
		}
	}
	return nil, false
}

// isGateAction 判断动作是否为「早期门禁类」（必须在缓存查找前执行、完整求值跳过）。
// 参数 a：策略动作。
// 返回 bool：auth / rate_limit 返回 true。
func isGateAction(a Action) bool {
	return a == ActionAuth || a == ActionRateLimit
}
