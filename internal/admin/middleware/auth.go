package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/auth"
	"gowaf/internal/admin/response"
	"gowaf/internal/config"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// isSafeMethod 判断 HTTP 方法是否为安全方法（GET/HEAD/OPTIONS），安全方法
// 不做 CSRF 校验、不进演示只读拦截、不记审计。
// 参数 m：HTTP 方法名。
// 返回：安全方法返回 true，写方法返回 false。
func isSafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// Auth 创建 JWT 认证中间件：校验令牌并把用户身份写入 Gin 上下文。
// 令牌优先取 Authorization 头（裸 token 或 Bearer 前缀均可），回落到 Cookie。
// 参数 mgr：令牌管理器，负责签名与声明校验。
// 返回：Gin 中间件；未登录、令牌过期或用途不符时返回 401 并终止请求。
func Auth(mgr *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 依次尝试 Authorization 头、Cookie、查询参数三种通道取令牌
		raw, fromCookie := extractToken(c)
		if raw == "" {
			response.Unauthorized(c, "未登录")
			return
		}
		claims, err := mgr.Parse(raw)
		if err != nil {
			response.Unauthorized(c, "登录已过期")
			return
		}
		// 拒绝用刷新令牌直接访问业务接口
		if claims.Purpose != auth.PurposeAccess {
			response.Unauthorized(c, "令牌类型错误")
			return
		}
		// 身份信息写入上下文，供后续中间件与 handler 使用
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUserName, claims.UserName)
		c.Set(CtxRole, claims.Role)
		if fromCookie {
			c.Set(CtxAuthFrom, "cookie")
		} else {
			c.Set(CtxAuthFrom, "header")
		}
		c.Next()
	}
}

// extractToken 取令牌。前端写的是 headers.set('Authorization', accessToken)，
// 不带 Bearer 前缀，这里两种都兼容。
//
// 第三个兜底通道是查询参数 `?token=`，专供 EventSource / <a download> 这类
// 无法自定义请求头的场景（AccessLog 只记录 Path 不记录 Query，不会泄漏令牌）。
// 参数 c：Gin 上下文。
// 返回：token 为取到的令牌（可能为空串）；fromCookie 表示令牌是否来自
// Cookie（CSRF 中间件据此决定是否执行双提交校验）。
func extractToken(c *gin.Context) (token string, fromCookie bool) {
	if h := c.GetHeader("Authorization"); h != "" {
		if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:]), false
		}
		return strings.TrimSpace(h), false
	}
	if ck, err := c.Cookie("waf_token"); err == nil && ck != "" {
		return ck, true
	}
	if q := strings.TrimSpace(c.Query("token")); q != "" {
		return q, false
	}
	return "", false
}

// CSRF 双提交校验。
//
// 只对「凭 Cookie 认证」的写请求生效：带 Authorization 头的请求需要 JS 主动设置
// 自定义头，浏览器不会在跨站请求里自动带上，本身免疫 CSRF。
// 参数 cfg：CSRF 配置（开关、Cookie 名与请求头名）。
// 返回：Gin 中间件；校验失败时返回 403 并终止请求。
func CSRF(cfg config.CSRF) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled || isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if c.GetString(CtxAuthFrom) != "cookie" {
			c.Next()
			return
		}
		// 双提交校验：Cookie 中保存的令牌必须与请求头携带的令牌一致，
		// 使用恒定时间比较避免时序侧信道
		want, _ := c.Cookie(cfg.CookieName)
		got := c.GetHeader(cfg.Header)
		if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
			response.Forbidden(c, "CSRF 校验失败，请刷新页面后重试")
			return
		}
		c.Next()
	}
}

// demoWhitelist 演示模式下仍放行的写接口：登录/登出/刷新令牌与规则测试，
// 这些操作不修改持久化数据。
var demoWhitelist = map[string]struct{}{
	"/api/auth/login":   {},
	"/api/auth/logout":  {},
	"/api/auth/refresh": {},
	"/api/rules/test":   {},
}

// DemoGuard 创建演示模式只读拦截中间件：开启后拦截所有写请求。
// 参数 readOnly：是否开启只读（演示）模式。
// 返回：Gin 中间件；被拦截的写请求返回 403（业务码 40301），白名单接口除外。
func DemoGuard(readOnly bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !readOnly || isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		// 登录/登出等白名单接口不受只读限制
		if _, ok := demoWhitelist[c.FullPath()]; ok {
			c.Next()
			return
		}
		response.Fail(c, http.StatusForbidden, response.CodeDemoReadOnly, "演示环境已禁用该操作")
	}
}

// RequirePerm 创建按钮级权限校验中间件：非 admin 角色必须在用户的 buttons
// 列表中持有指定权限码，前端隐藏按钮只是体验优化，后端必须再挡一次。
// 参数 st：存储层，用于按用户 ID 加载用户记录。
// 参数 perm：权限码，如 "ip:ban"、"rule:write"。
// 返回：Gin 中间件；账号不存在或缺少权限时返回 403 并终止请求。
func RequirePerm(st *store.Store, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString(CtxRole)
		// admin 角色拥有全部按钮权限，直接放行
		if role == "admin" {
			c.Next()
			return
		}
		uid := c.GetUint(CtxUserID)
		var u model.User
		if err := st.Gorm().First(&u, uid).Error; err != nil {
			response.Forbidden(c, "账号不存在")
			return
		}
		// buttons 字段在数据库中保存为 JSON 数组字符串
		var buttons []string
		_ = json.Unmarshal([]byte(u.Buttons), &buttons)
		for _, b := range buttons {
			if b == perm {
				c.Next()
				return
			}
		}
		response.Forbidden(c, "缺少权限: "+perm)
	}
}

// LoginLimiter 按「IP + 用户名」维度做登录失败锁定，防止口令暴力破解。
// 内部状态由互斥锁保护，可安全地被并发请求调用。
type LoginLimiter struct {
	mu      sync.Mutex             // 保护 entries 的互斥锁。
	entries map[string]*loginEntry // 失败记录表，键为 "ip|username"（用户名统一转小写）。
	maxFail int                    // 连续失败达到该次数即锁定。
	lockFor time.Duration          // 锁定时长。
}

// loginEntry 记录单个「IP + 用户名」组合的失败计数与锁定状态。
type loginEntry struct {
	fails       int       // 连续失败次数，达到阈值后清零并进入锁定。
	lockedUntil time.Time // 锁定截止时间，早于当前时间表示未锁定。
	last        time.Time // 最近一次失败时间，用于过期清理。
}

// NewLoginLimiter 创建登录限速器，非法参数取内置默认值。
// 参数 maxFail：允许的连续失败次数上限，<=0 时默认 5 次。
// 参数 lockFor：锁定时长，<=0 时默认 15 分钟。
// 返回：可并发使用的登录限速器。
func NewLoginLimiter(maxFail int, lockFor time.Duration) *LoginLimiter {
	if maxFail <= 0 {
		maxFail = 5
	}
	if lockFor <= 0 {
		lockFor = 15 * time.Minute
	}
	return &LoginLimiter{entries: make(map[string]*loginEntry), maxFail: maxFail, lockFor: lockFor}
}

// Allowed 判断指定 IP + 用户名当前是否允许再次尝试登录。
// 参数 ip：客户端 IP；userName：登录用户名（不区分大小写）。
// 返回：首个值为是否允许尝试；第二个值为仍在锁定时的剩余秒数（未锁定时为 0）。
func (l *LoginLimiter) Allowed(ip, userName string) (bool, int) {
	key := ip + "|" + strings.ToLower(userName)
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true, 0
	}
	if time.Now().Before(e.lockedUntil) {
		return false, int(time.Until(e.lockedUntil).Seconds())
	}
	return true, 0
}

// Fail 记录一次登录失败；连续失败达到阈值则进入锁定期并清零计数。
// 参数 ip：客户端 IP；userName：登录用户名（不区分大小写）。
// 返回：无。
func (l *LoginLimiter) Fail(ip, userName string) {
	key := ip + "|" + strings.ToLower(userName)
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		e = &loginEntry{}
		l.entries[key] = e
	}
	e.fails++
	e.last = time.Now()
	// 达到阈值：进入锁定期，同时清零计数，锁定结束后重新累计
	if e.fails >= l.maxFail {
		e.lockedUntil = time.Now().Add(l.lockFor)
		e.fails = 0
	}
}

// Reset 登录成功后清除该 IP + 用户名的全部失败记录。
// 参数 ip：客户端 IP；userName：登录用户名。
// 返回：无。
func (l *LoginLimiter) Reset(ip, userName string) {
	key := ip + "|" + strings.ToLower(userName)
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

// Cleanup 清理长时间不活跃且已解除锁定的条目，防止 entries 无限增长。
// 参数：无。
// 返回：本次清理删除的条目数。
func (l *LoginLimiter) Cleanup() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	removed := 0
	// 最近 2 小时内无失败记录且已过锁定期的条目视为过期
	cutoff := time.Now().Add(-2 * time.Hour)
	for k, e := range l.entries {
		if e.last.Before(cutoff) && time.Now().After(e.lockedUntil) {
			delete(l.entries, k)
			removed++
		}
	}
	return removed
}

// LoginLimit 创建登录接口的限流中间件，按 IP 粗粒度预检。
// 参数 l：登录限速器，此处仅按 IP 维度检查（用户名固定传 "-"），
// 用户名维度的精确限制在登录 handler 内调用 Allowed/Fail 完成。
// 返回：Gin 中间件；超过频率限制时返回 429 并提示剩余等待秒数。
func LoginLimit(l *LoginLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := ClientIP(c)
		if ok, wait := l.Allowed(ip, "-"); !ok {
			response.Fail(c, http.StatusTooManyRequests, response.CodeTooMany,
				fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", wait))
			return
		}
		c.Next()
	}
}

// Audit 创建审计日志中间件：在 handler 执行完毕后记录写操作，
// handler 可通过 CtxAuditObj / CtxAuditID 细化审计对象。
// 参数 st：存储层，用于落库审计记录。
// 返回：Gin 中间件；安全方法与 5xx 请求不记录，401/403 也会留痕。
func Audit(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if isSafeMethod(c.Request.Method) {
			return
		}
		status := c.Writer.Status()
		// 401/403 也要留痕（越权尝试是重要信号），5xx 不记
		if status >= 500 {
			return
		}

		// handler 未显式指定审计对象时，按路由路径推断
		object := c.GetString(CtxAuditObj)
		if object == "" {
			object = guessObject(c.FullPath())
		}
		result := "ok"
		if status >= 400 {
			result = "deny"
		}

		operator := c.GetString(CtxUserName)
		if operator == "" {
			operator = "anonymous"
		}

		st.Audit(model.AuditLog{
			TS:       store.NowMS(),
			Operator: operator,
			ClientIP: ClientIP(c),
			Action:   strings.ToLower(c.Request.Method),
			Object:   object,
			ObjectID: c.GetString(CtxAuditID),
			After:    c.Request.URL.Path,
			Result:   result,
		})
	}
}

// guessObject 从路由路径推断审计对象类型。
// 参数 fullPath：Gin 路由全路径，如 /api/ips。
// 返回：对象名（如 ip_rule、ban、rule、site）；无法识别时返回 unknown 或路径首段。
func guessObject(fullPath string) string {
	parts := strings.Split(strings.TrimPrefix(fullPath, "/api/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "unknown"
	}
	switch parts[0] {
	case "ips":
		return "ip_rule"
	case "bans":
		return "ban"
	case "rules":
		return "rule"
	case "sites":
		return "site"
	case "settings":
		return "settings"
	case "auth":
		return "auth"
	case "user":
		return "user"
	}
	return parts[0]
}
