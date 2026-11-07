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

// 上下文键常量：中间件与 handler 之间通过 c.Set/c.Get 传递数据的键名。
// CtxUserID/CtxUserName/CtxRole 由 Auth 写入；CtxAuthFrom 标记令牌来源
// （header 或 cookie），CSRF 据此决定是否校验；CtxReqID 由 RequestID 写入；
// CtxAuditObj/CtxAuditID 由 handler 写入，供 Audit 细化审计对象。
const (
	CtxUserID   = "ctx_uid"
	CtxUserName = "ctx_uname"
	CtxRole     = "ctx_role"
	CtxAuthFrom = "ctx_authfrom"
	CtxReqID    = "ctx_reqid"
	CtxAuditObj = "ctx_audit_obj"
	CtxAuditID  = "ctx_audit_id"
)

// Recover 创建 panic 恢复中间件，避免控制面被单个请求的 panic 打挂。
// 参数：无。
// 返回：Gin 中间件；捕获 panic 时记录日志与调用栈，并返回 500。
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

// RequestID 创建请求 ID 中间件：优先复用客户端带来的 X-Request-ID 头，
// 否则以纳秒时间戳生成一个。
// 参数：无。
// 返回：Gin 中间件；请求 ID 同时写入上下文（CtxReqID）与响应头。
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

// AccessLog 创建访问日志中间件，只记录慢请求与错误响应，避免正常流量刷屏。
// 参数：无。
// 返回：Gin 中间件；请求耗时超过 500ms 或响应状态码 >=400 时输出一条日志。
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

// CORS 创建跨域中间件（手写实现而非引库），精确处理「带凭证时不能回
// 通配符 *」与 OPTIONS 预检短路两个易错点。
// 参数 cfg：CORS 配置（允许的来源/方法/头、是否带凭证、预检缓存时长）。
// 参数 demo：是否演示模式，演示模式下若不带凭证则放行任意来源只读访问。
// 返回：Gin 中间件；来源不在白名单时不回任何 CORS 头，由浏览器自行拦截。
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

		// 来源精确匹配（大小写不敏感），绝不回通配符 *
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

// IPAllowlist 创建来源 IP 白名单中间件，只允许配置网段访问控制面；空配置表示不限制。
// 参数 cidrs：允许的网段列表，支持 CIDR（如 10.0.0.0/8）与单 IP 两种写法。
// 返回：Gin 中间件；来源地址无法解析或不在白名单内时返回 403。
func IPAllowlist(cidrs []string) gin.HandlerFunc {
	var list []netip.Prefix
	// 逐条解析白名单：先按 CIDR 解析，失败再按单个 IP 解析（等价于全位掩码网段）
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

// parseAddr 解析 IP 或「IP:端口」形式的地址字符串。
// 参数 s：待解析的地址。
// 返回：解析出的 IP（IPv4-mapped IPv6 已还原为 IPv4）与是否解析成功。
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

// ClientIP 返回客户端 IP：优先取 SetRealIP 预先写入上下文的真实 IP
// （已结合可信代理计算），否则退回 Gin 自身的解析结果。
// 参数 c：Gin 上下文。
// 返回：客户端 IP 字符串。
func ClientIP(c *gin.Context) string {
	if v, ok := c.Get("real_ip"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return c.ClientIP()
}

// SetRealIP 在请求链路最早阶段把真实客户端 IP 写入上下文（由 router
// 结合可信代理配置计算），后续统一经 ClientIP 读取。
// 参数 c：Gin 上下文；ip：解析后的真实客户端 IP。
// 返回：无。
func SetRealIP(c *gin.Context, ip string) {
	c.Set("real_ip", ip)
}
