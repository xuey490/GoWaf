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

// 安全方法无需 CSRF 校验。
func isSafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// Auth 校验 JWT。优先 Authorization 头（裸 token 或 Bearer 均可），回落到 Cookie。
func Auth(mgr *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
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
		if claims.Purpose != auth.PurposeAccess {
			response.Unauthorized(c, "令牌类型错误")
			return
		}
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
		want, _ := c.Cookie(cfg.CookieName)
		got := c.GetHeader(cfg.Header)
		if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
			response.Forbidden(c, "CSRF 校验失败，请刷新页面后重试")
			return
		}
		c.Next()
	}
}

// demoWhitelist 演示模式下仍放行的写接口（不改数据）。
var demoWhitelist = map[string]struct{}{
	"/api/auth/login":   {},
	"/api/auth/logout":  {},
	"/api/auth/refresh": {},
	"/api/rules/test":   {},
}

// DemoGuard 演示模式只读拦截。
func DemoGuard(readOnly bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !readOnly || isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if _, ok := demoWhitelist[c.FullPath()]; ok {
			c.Next()
			return
		}
		response.Fail(c, http.StatusForbidden, response.CodeDemoReadOnly, "演示环境已禁用该操作")
	}
}

// RequirePerm 校验按钮权限。前端 disabled 只是体验，后端必须再挡一次。
func RequirePerm(st *store.Store, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString(CtxRole)
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

// LoginLimiter 按 IP + 用户名做登录失败锁定。
type LoginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginEntry
	maxFail int
	lockFor time.Duration
}

type loginEntry struct {
	fails       int
	lockedUntil time.Time
	last        time.Time
}

// NewLoginLimiter 创建登录限速器。
func NewLoginLimiter(maxFail int, lockFor time.Duration) *LoginLimiter {
	if maxFail <= 0 {
		maxFail = 5
	}
	if lockFor <= 0 {
		lockFor = 15 * time.Minute
	}
	return &LoginLimiter{entries: make(map[string]*loginEntry), maxFail: maxFail, lockFor: lockFor}
}

// Allowed 判断是否允许尝试，返回剩余锁定秒数。
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

// Fail 记一次失败。
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
	if e.fails >= l.maxFail {
		e.lockedUntil = time.Now().Add(l.lockFor)
		e.fails = 0
	}
}

// Reset 登录成功后清零。
func (l *LoginLimiter) Reset(ip, userName string) {
	key := ip + "|" + strings.ToLower(userName)
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

// Cleanup 清理过期条目。
func (l *LoginLimiter) Cleanup() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	removed := 0
	cutoff := time.Now().Add(-2 * time.Hour)
	for k, e := range l.entries {
		if e.last.Before(cutoff) && time.Now().After(e.lockedUntil) {
			delete(l.entries, k)
			removed++
		}
	}
	return removed
}

// LoginLimit 登录接口的限流中间件（仅按 IP 粗挡，用户维度在 handler 内精确判断）。
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

// Audit 记录写操作。handler 可通过 CtxAuditObj / CtxAuditID 细化对象。
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

// guessObject 从路由推断审计对象。
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
