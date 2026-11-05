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

		// 需登录
		authed := api.Group("", middleware.Auth(o.Auth))
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
			authed.GET("/rules", h.RuleList)
			authed.GET("/rules/groups", h.RuleGroups)
			authed.GET("/rules/stats", h.RuleStats)
			authed.GET("/rules/export", h.RuleExport)
			authed.GET("/rules/:id/samples", h.RuleSamples)
			authed.GET("/exceptions", h.ExceptionList)
			authed.GET("/intel/sources", h.IntelList)
			authed.GET("/intel/status", h.IntelStatus)
			authed.GET("/intel/presets", h.IntelPresets)
			authed.GET("/geo/status", h.GeoStatus)
			authed.GET("/notify/logs", h.NotifyLogs)
			authed.GET("/sites", h.SiteList)
			authed.GET("/settings", h.SettingsGet)
			authed.GET("/settings/versions", h.SettingsVersions)
			authed.GET("/audit", h.AuditList)

			// 规则测试是纯计算，不进写组（演示模式下也要能用）
			authed.POST("/rules/test", h.RuleTest)

			// 写操作：CSRF → DemoGuard → 审计 → 权限
			// 顺序含义：先做 CSRF 双提交校验，再拦演示只读，随后记录审计，
			// 最后按路由逐个做按钮级权限校验
			w := authed.Group("", middleware.CSRF(o.Cfg.Admin.CSRF),
				middleware.DemoGuard(o.Cfg.Demo.Enabled && o.Cfg.Demo.ReadOnly),
				middleware.Audit(o.Store))
			{
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

				w.PUT("/settings", middleware.RequirePerm(o.Store, "settings:write"), h.SettingsUpdate)
				w.POST("/settings/reload", middleware.RequirePerm(o.Store, "settings:write"), h.SettingsReload)
				w.POST("/settings/rollback", middleware.RequirePerm(o.Store, "settings:write"), h.SettingsRollback)

				authed.GET("/tasks", h.TasksList)
				w.PUT("/tasks/:name", middleware.RequirePerm(o.Store, "settings:write"), h.TaskUpdate)
				w.POST("/tasks/:name/run", middleware.RequirePerm(o.Store, "settings:write"), h.TaskRun)
			}
		}
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
