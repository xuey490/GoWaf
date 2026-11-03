// Package middleware 控制面中间件。
//
// 装配顺序（不可随意调整）：
// Recover → RequestID → AccessLog → CORS → IPAllowlist
//
//	→ LoginLimit(仅登录) → Auth → CSRF → DemoGuard → Audit → Handler
package middleware

import (
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
	"gowaf/internal/config"
)

// 上下文键。
const (
	CtxUserID   = "ctx_uid"
	CtxUserName = "ctx_uname"
	CtxRole     = "ctx_role"
	CtxAuthFrom = "ctx_authfrom"
	CtxReqID    = "ctx_reqid"
	CtxAuditObj = "ctx_audit_obj"
	CtxAuditID  = "ctx_audit_id"
)

// Recover 捕获 panic，避免控制面被单个请求打挂。
func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[admin] panic: %v\n%s", r, debug.Stack())
				response.Internal(c, "服务内部错误")
			}
		}()
		c.Next()
	}
}

// RequestID 生成并回写请求 ID。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		c.Set(CtxReqID, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// AccessLog 只记录慢请求与错误，避免刷屏。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		cost := time.Since(start)
		if cost > 500*time.Millisecond || c.Writer.Status() >= 400 {
			log.Printf("[admin] %s %s -> %d (%s) ip=%s",
				c.Request.Method, c.Request.URL.Path, c.Writer.Status(), cost, c.ClientIP())
		}
	}
}

// CORS 手写实现，精确处理「带凭证时不能用 *」与预检短路。
func CORS(cfg config.CORS, demo bool) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(cfg.AllowOrigins))
	for _, o := range cfg.AllowOrigins {
		allowed[strings.ToLower(strings.TrimSpace(o))] = struct{}{}
	}
	methods := strings.Join(cfg.AllowMethods, ", ")
	headers := strings.Join(cfg.AllowHeaders, ", ")
	expose := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(int(cfg.MaxAge.Duration().Seconds()))

	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		_, ok := allowed[strings.ToLower(origin)]
		// 演示模式且不带凭证时，允许任意来源只读访问
		if !ok && !(demo && !cfg.AllowCredentials) {
			// 不回 CORS 头，让浏览器自己拦，服务端不报错
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Vary", "Origin")
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Methods", methods)
		h.Set("Access-Control-Allow-Headers", headers)
		if expose != "" {
			h.Set("Access-Control-Expose-Headers", expose)
		}
		if maxAge != "0" {
			h.Set("Access-Control-Max-Age", maxAge)
		}
		if cfg.AllowCredentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}

		// 预检必须在这里结束，否则会被 Auth 拦成 401，浏览器只看到跨域错误
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// IPAllowlist 只允许配置的网段访问控制面；空配置表示不限制。
func IPAllowlist(cidrs []string) gin.HandlerFunc {
	var list []netip.Prefix
	for _, s := range cidrs {
		if p, err := netip.ParsePrefix(s); err == nil {
			list = append(list, p)
			continue
		}
		if a, err := netip.ParseAddr(s); err == nil {
			list = append(list, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return func(c *gin.Context) {
		if len(list) == 0 {
			c.Next()
			return
		}
		addr, ok := parseAddr(c.ClientIP())
		if !ok {
			response.Forbidden(c, "来源地址无法识别")
			return
		}
		for _, p := range list {
			if p.Contains(addr) {
				c.Next()
				return
			}
		}
		response.Forbidden(c, "IP 不在允许列表内")
	}
}

func parseAddr(s string) (netip.Addr, bool) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// ClientIP 返回解析后的客户端 IP 字符串。
func ClientIP(c *gin.Context) string {
	if v, ok := c.Get("real_ip"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return c.ClientIP()
}

// SetRealIP 由 router 在最早阶段写入真实 IP（考虑可信代理）。
func SetRealIP(c *gin.Context, ip string) {
	c.Set("real_ip", ip)
}
