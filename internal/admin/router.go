// Package admin 装配管理后台（控制面）的 HTTP 路由：注册全局中间件链、
// 公开/鉴权/写操作分组接口、SSE 推送以及前端静态资源。
package admin

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"gowaf/internal/admin/auth"
	"gowaf/internal/admin/handler"
	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/config"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
	"gowaf/internal/web"
)

// Options 路由装配所需的全部依赖，由应用入口在启动时注入。
type Options struct {
	Cfg     *config.Config           // 全局配置（CORS、CSRF、IP 白名单、演示模式等）。
	Store   *store.Store             // 存储层，供审计与权限校验查询用户。
	RT      *runtime.Runtime         // WAF 运行时句柄。
	Auth    *auth.Manager            // JWT 令牌管理器。
	Login   *middleware.LoginLimiter // 登录失败限速器。
	Handler *handler.Server          // 业务 handler 集合。
	Version string                   // 控制面版本号。
}

// NewRouter 构建 Gin 引擎并注册控制面全部路由。
// 参数 o：装配参数，携带配置、存储与各业务依赖。
// 返回：配置完成的 Gin 引擎（由调用方负责启动监听）。
func NewRouter(o Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 控制面永远不挂在反向代理后面（默认只监听回环，临时外放也是直连 IP），
	// 所以不信任任何代理头：gin 默认信任全部来源，任何人加一个
	// "X-Forwarded-For: 127.0.0.1" 就能绕过 admin.allowed_ips 白名单和登录失败计数。
	_ = r.SetTrustedProxies(nil)

	// 全局中间件链：panic 恢复 → 请求 ID → 慢/错日志 → CORS → IP 白名单
	r.Use(middleware.Recover(), middleware.RequestID(), middleware.AccessLog())
	r.Use(middleware.CORS(o.Cfg.Admin.CORS, o.Cfg.Demo.Enabled))
	r.Use(middleware.IPAllowlist(o.Cfg.Admin.AllowedIPs))

	h := o.Handler

	// 健康检查与指标（不放 /api 下，避免被鉴权挡住）
	r.GET("/healthz", h.Healthz)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// /api 分组：REST 接口，按「公开 → 需登录 → 写操作」三层组织
	api := r.Group("/api")
	{
		// 公开接口
		api.POST("/auth/login", middleware.LoginLimit(o.Login), h.Login)
		api.POST("/auth/logout", h.Logout)
		api.POST("/auth/refresh", h.Refresh)

		// 节点 Agent 上报：不套登录态（Agent 没有后台会话），
		// 由 handler 内部校验设置表里的 node.agent_token。
		api.GET("/agent/config", h.AgentConfig)
		api.POST("/agent/heartbeat", h.AgentHeartbeat)

		// 需登录。API Token（Bearer）先行尝试：命中则跳过 JWT（见 APITokenAuth），
		// 未命中再走 JWT/Cookie 流程——OpenAPI 与 MCP 客户端由此免会话访问。
		authed := api.Group("", middleware.APITokenAuth(o.Store), middleware.Auth(o.Auth))
		{
			authed.GET("/auth/csrf", h.CSRFToken)
			authed.GET("/user/info", h.Info)
			authed.PUT("/user/password", h.UpdatePassword)
			authed.GET("/user/list", h.UserList)
			authed.GET("/system/info", h.SystemInfo)
			authed.GET("/system/debug", h.MetricsDebug)

			authed.GET("/dashboard/overview", h.Overview)
			authed.GET("/dashboard/timeseries", h.Timeseries)
			authed.GET("/dashboard/top", h.Top)
			authed.GET("/dashboard/categories", h.Categories)

			authed.GET("/events", h.EventList)
			authed.GET("/events/export", h.EventExport)
			authed.GET("/events/:id", h.EventDetail)

			authed.GET("/ips", h.IPList)
			authed.GET("/bans", h.BanList)
			// 封禁详情里的「关联拦截事件」：按封禁记录反查触发它的事件
			authed.GET("/bans/:id/events", h.BanEvents)
			authed.GET("/rules", h.RuleList)
			authed.GET("/rules/groups", h.RuleGroups)
			authed.GET("/rules/stats", h.RuleStats)
			authed.GET("/rules/export", h.RuleExport)
			authed.GET("/rules/:id/samples", h.RuleSamples)
			authed.GET("/exceptions", h.ExceptionList)

			// 第一批~第三批扩展能力：精准访问控制 / Bot / 页面缓存 / 节点 / 多上游
			authed.GET("/policies", h.PolicyList)
			authed.GET("/bots", h.BotList)
			authed.GET("/bot-honeypots", h.HoneypotList)
			authed.GET("/cache/rules", h.CacheRuleList)
			authed.GET("/cache/settings", h.CacheSettingsGet)
			authed.GET("/cache/stats", h.CacheStats)
			authed.GET("/cache/entries", h.CacheEntryList)
			authed.GET("/nodes", h.NodeList)
			authed.GET("/upstreams/status", h.UpstreamStatus)

			authed.GET("/intel/sources", h.IntelList)
			authed.GET("/intel/status", h.IntelStatus)
			authed.GET("/intel/presets", h.IntelPresets)
			authed.GET("/geo/status", h.GeoStatus)
			authed.GET("/crs/check-update", h.CRSCheckUpdate)
			authed.GET("/notify/logs", h.NotifyLogs)
			authed.GET("/sites", h.SiteList)
			authed.GET("/sites/:id/upstreams", h.SiteUpstreamList)
			authed.GET("/settings", h.SettingsGet)
			authed.GET("/settings/versions", h.SettingsVersions)
			authed.GET("/audit", h.AuditList)
			authed.GET("/tokens", h.TokenList)

			// 证书管理（本地证书 + ACME 签发订单）
			authed.GET("/certs", h.CertList)
			authed.GET("/certs/acme/orders", h.AcmeOrders)
			authed.GET("/tamper/status", h.TamperStatus)

			// 规则测试是纯计算，不进写组（演示模式下也要能用）
			authed.POST("/rules/test", h.RuleTest)

			// 写操作：API Token 作用域 → CSRF → DemoGuard → 审计 → 权限
			// 顺序含义：read-only 令牌先被 RequireAPIScope 挡掉（JWT 会话不受影响），
			// 再做 CSRF 双提交校验（令牌走 Authorization 头天然免 CSRF），
			// 随后拦演示只读、记录审计，最后按路由逐个做按钮级权限校验
			w := authed.Group("", middleware.RequireAPIScope(middleware.ScopeWrite),
				middleware.CSRF(o.Cfg.Admin.CSRF),
				middleware.DemoGuard(o.Cfg.Demo.Enabled && o.Cfg.Demo.ReadOnly),
				middleware.Audit(o.Store))
			{
				// API 令牌管理（令牌本身即凭据，管理动作仍要求 settings:write 会话权限）
				w.POST("/tokens", middleware.RequirePerm(o.Store, "settings:write"), h.TokenCreate)
				w.DELETE("/tokens/:id", middleware.RequirePerm(o.Store, "settings:write"), h.TokenRevoke)

				// 证书管理与 ACME 签发
				// 删除路由用 /certs/domain/:domain 前缀：gin 路由树不允许同级
				// 既有 :domain 参数段又有 acme 静态段，加一段前缀让两者共存。
				w.POST("/certs/upload", middleware.RequirePerm(o.Store, "settings:write"), h.CertUpload)
				w.POST("/certs/reload", middleware.RequirePerm(o.Store, "settings:write"), h.CertReload)
				w.DELETE("/certs/domain/:domain", middleware.RequirePerm(o.Store, "settings:write"), h.CertDelete)
				w.POST("/certs/acme/order", middleware.RequirePerm(o.Store, "settings:write"), h.AcmeCreateOrder)
				w.POST("/certs/acme/order/:id/validate", middleware.RequirePerm(o.Store, "settings:write"), h.AcmeValidateOrder)
				w.DELETE("/certs/acme/order/:id", middleware.RequirePerm(o.Store, "settings:write"), h.AcmeCancelOrder)

				// 防篡改基线
				w.POST("/tamper/refresh", middleware.RequirePerm(o.Store, "settings:write"), h.TamperRefresh)

				w.POST("/ips", middleware.RequirePerm(o.Store, "ip:ban"), h.IPCreate)
				w.PUT("/ips/:id", middleware.RequirePerm(o.Store, "ip:ban"), h.IPUpdate)
				w.DELETE("/ips/:id", middleware.RequirePerm(o.Store, "ip:ban"), h.IPDelete)
				w.POST("/ips/import", middleware.RequirePerm(o.Store, "ip:ban"), h.IPImport)

				w.POST("/bans", middleware.RequirePerm(o.Store, "ip:ban"), h.BanCreate)
				w.DELETE("/bans/:id", middleware.RequirePerm(o.Store, "ip:ban"), h.BanDelete)
				w.DELETE("/bans", middleware.RequirePerm(o.Store, "ip:ban"), h.BanClear)

				w.POST("/rules", middleware.RequirePerm(o.Store, "rule:write"), h.RuleCreate)
				w.PUT("/rules/:id", middleware.RequirePerm(o.Store, "rule:write"), h.RuleUpdate)
				w.PATCH("/rules/:id/toggle", middleware.RequirePerm(o.Store, "rule:write"), h.RuleToggle)
				w.POST("/rules/reload", middleware.RequirePerm(o.Store, "rule:write"), h.RuleReload)
				w.POST("/rules/batch", middleware.RequirePerm(o.Store, "rule:write"), h.RuleBatch)
				w.POST("/rules/import", middleware.RequirePerm(o.Store, "rule:write"), h.RuleImport)
				w.GET("/seclang/packages", h.SecLangPackages)
				w.POST("/seclang/import", middleware.RequirePerm(o.Store, "rule:write"), h.SecLangImport)
				w.POST("/seclang/:id/toggle", middleware.RequirePerm(o.Store, "rule:write"), h.SecLangToggle)
				w.DELETE("/seclang/:id", middleware.RequirePerm(o.Store, "rule:write"), h.SecLangDelete)

				w.POST("/exceptions", middleware.RequirePerm(o.Store, "rule:write"), h.ExceptionCreate)
				w.PUT("/exceptions/:id", middleware.RequirePerm(o.Store, "rule:write"), h.ExceptionUpdate)
				w.PATCH("/exceptions/:id/toggle", middleware.RequirePerm(o.Store, "rule:write"), h.ExceptionToggle)
				w.DELETE("/exceptions/:id", middleware.RequirePerm(o.Store, "rule:write"), h.ExceptionDelete)

				w.POST("/intel/sources", middleware.RequirePerm(o.Store, "ip:ban"), h.IntelCreate)
				w.PUT("/intel/sources/:id", middleware.RequirePerm(o.Store, "ip:ban"), h.IntelUpdate)
				w.DELETE("/intel/sources/:id", middleware.RequirePerm(o.Store, "ip:ban"), h.IntelDelete)
				w.POST("/intel/sync", middleware.RequirePerm(o.Store, "ip:ban"), h.IntelSync)
				w.POST("/intel/presets", middleware.RequirePerm(o.Store, "ip:ban"), h.IntelPresetAdd)

				w.POST("/geo/backfill", middleware.RequirePerm(o.Store, "settings:write"), h.GeoBackfill)
				w.POST("/notify/test", middleware.RequirePerm(o.Store, "settings:write"), h.NotifyTest)
				w.DELETE("/rules/:id", middleware.RequirePerm(o.Store, "rule:delete"), h.RuleDelete)

				w.POST("/sites", middleware.RequirePerm(o.Store, "site:write"), h.SiteCreate)
				w.PUT("/sites/:id", middleware.RequirePerm(o.Store, "site:write"), h.SiteUpdate)
				w.DELETE("/sites/:id", middleware.RequirePerm(o.Store, "site:write"), h.SiteDelete)
				w.POST("/sites/:id/test", middleware.RequirePerm(o.Store, "site:write"), h.SiteTest)

				// 精准访问控制
				w.POST("/policies", middleware.RequirePerm(o.Store, "policy:write"), h.PolicyCreate)
				w.PUT("/policies/:id", middleware.RequirePerm(o.Store, "policy:write"), h.PolicyUpdate)
				w.PATCH("/policies/:id/toggle", middleware.RequirePerm(o.Store, "policy:write"), h.PolicyToggle)
				w.DELETE("/policies/:id", middleware.RequirePerm(o.Store, "policy:write"), h.PolicyDelete)

				// Bot 管理
				w.POST("/bots", middleware.RequirePerm(o.Store, "bot:write"), h.BotCreate)
				w.PUT("/bots/:id", middleware.RequirePerm(o.Store, "bot:write"), h.BotUpdate)
				w.PATCH("/bots/:id/toggle", middleware.RequirePerm(o.Store, "bot:write"), h.BotToggle)
				w.DELETE("/bots/:id", middleware.RequirePerm(o.Store, "bot:write"), h.BotDelete)
				w.POST("/bot-honeypots", middleware.RequirePerm(o.Store, "bot:write"), h.HoneypotCreate)
				w.PUT("/bot-honeypots/:id", middleware.RequirePerm(o.Store, "bot:write"), h.HoneypotUpdate)
				w.PATCH("/bot-honeypots/:id/toggle", middleware.RequirePerm(o.Store, "bot:write"), h.HoneypotToggle)
				w.DELETE("/bot-honeypots/:id", middleware.RequirePerm(o.Store, "bot:write"), h.HoneypotDelete)

				// 页面缓存
				w.POST("/cache/rules", middleware.RequirePerm(o.Store, "cache:write"), h.CacheRuleCreate)
				w.PUT("/cache/rules/:id", middleware.RequirePerm(o.Store, "cache:write"), h.CacheRuleUpdate)
				w.PATCH("/cache/rules/:id/toggle", middleware.RequirePerm(o.Store, "cache:write"), h.CacheRuleToggle)
				w.DELETE("/cache/rules/:id", middleware.RequirePerm(o.Store, "cache:write"), h.CacheRuleDelete)
				w.PUT("/cache/settings", middleware.RequirePerm(o.Store, "cache:write"), h.CacheSettingsUpdate)
				w.POST("/cache/purge", middleware.RequirePerm(o.Store, "cache:write"), h.CachePurge)

				// 节点与站点多上游
				w.POST("/nodes", middleware.RequirePerm(o.Store, "node:write"), h.NodeCreate)
				w.PUT("/nodes/:id", middleware.RequirePerm(o.Store, "node:write"), h.NodeUpdate)
				w.DELETE("/nodes/:id", middleware.RequirePerm(o.Store, "node:write"), h.NodeDelete)
				w.POST("/nodes/token", middleware.RequirePerm(o.Store, "node:write"), h.NodeToken)
				w.PUT("/sites/:id/upstreams", middleware.RequirePerm(o.Store, "site:write"), h.SiteUpstreamSave)

				w.PUT("/settings", middleware.RequirePerm(o.Store, "settings:write"), h.SettingsUpdate)
				w.POST("/settings/reload", middleware.RequirePerm(o.Store, "settings:write"), h.SettingsReload)
				w.POST("/settings/rollback", middleware.RequirePerm(o.Store, "settings:write"), h.SettingsRollback)

				authed.GET("/tasks", h.TasksList)
				w.PUT("/tasks/:name", middleware.RequirePerm(o.Store, "settings:write"), h.TaskUpdate)
				w.POST("/tasks/:name/run", middleware.RequirePerm(o.Store, "settings:write"), h.TaskRun)
			}
		}
	}

	// OpenAPI 别名组 /api/v1：供外部集成与文档引用的稳定契约面。
	// 处理器与 /api 同名路由一一对应，凭 API Token（Bearer）或登录会话访问；
	// 写接口同样经 RequireAPIScope 挡 read-only 令牌。
	v1 := r.Group("/api/v1", middleware.APITokenAuth(o.Store), middleware.Auth(o.Auth))
	{
		v1.GET("/overview", h.Overview)
		v1.GET("/timeseries", h.Timeseries)
		v1.GET("/top", h.Top)
		v1.GET("/categories", h.Categories)
		v1.GET("/events", h.EventList)
		v1.GET("/events/:id", h.EventDetail)
		v1.GET("/sites", h.SiteList)
		v1.GET("/bans", h.BanList)
		v1.GET("/ips", h.IPList)
		v1.GET("/rules", h.RuleList)
		v1.GET("/settings", h.SettingsGet)
		v1.GET("/tokens", h.TokenList)
		v1.POST("/rules/test", h.RuleTest)
		v1.POST("/bans", middleware.RequireAPIScope(middleware.ScopeWrite), h.BanCreate)
		v1.DELETE("/bans/:id", middleware.RequireAPIScope(middleware.ScopeWrite), h.BanDelete)
		v1.POST("/tokens", middleware.RequireAPIScope(middleware.ScopeWrite), h.TokenCreate)
		v1.DELETE("/tokens/:id", middleware.RequireAPIScope(middleware.ScopeWrite), h.TokenRevoke)
	}

	// SSE 单独一组：不套 Audit（长连接会让审计中间件一直挂着），也不套 CSRF
	r.GET("/api/stream/events", middleware.Auth(o.Auth), h.EventStream)

	// 挂载内嵌前端静态资源，处理 history 路由回退
	mountSPA(r)
	return r
}

// mountSPA 挂载内嵌的前端静态资源，并处理 history 路由回退：
// 命中静态文件直接返回，其余路径一律回 index.html 交给前端路由。
// 参数 r：Gin 引擎。
// 返回：无（通过 NoRoute 注册处理逻辑）。
func mountSPA(r *gin.Engine) {
	sub, err := web.Sub()
	if err != nil {
		r.NoRoute(func(c *gin.Context) {
			response.NotFound(c, "前端资源未打包")
		})
		return
	}

	fileServer := http.FileServer(http.FS(sub))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// API 与内置端点的未知路径一律返回 404，不回退到前端页面
		if strings.HasPrefix(path, "/api/") || path == "/healthz" || path == "/metrics" {
			response.NotFound(c, "接口不存在")
			return
		}

		// 静态文件存在就直接返回
		clean := strings.TrimPrefix(path, "/")
		if clean != "" {
			if f, err := sub.Open(clean); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		// 其余一律回 index.html，交给前端路由
		serveIndex(c, sub)
	})
}

// serveIndex 返回前端入口页 index.html。
// 参数 c：Gin 上下文；sub：内嵌的前端文件系统。
// 返回：无（直接写出 HTML 响应；读取失败时返回 404 文本）。
func serveIndex(c *gin.Context, sub fs.FS) {
	body, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		c.String(http.StatusNotFound, "index.html 缺失")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", body)
}
