// Package admin 装配控制面路由。
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

// Options 装配参数。
type Options struct {
	Cfg     *config.Config
	Store   *store.Store
	RT      *runtime.Runtime
	Auth    *auth.Manager
	Login   *middleware.LoginLimiter
	Handler *handler.Server
	Version string
}

// NewRouter 构建 Gin 路由。
func NewRouter(o Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 控制面永远不挂在反向代理后面（默认只监听回环，临时外放也是直连 IP），
	// 所以不信任任何代理头：gin 默认信任全部来源，任何人加一个
	// "X-Forwarded-For: 127.0.0.1" 就能绕过 admin.allowed_ips 白名单和登录失败计数。
	_ = r.SetTrustedProxies(nil)

	r.Use(middleware.Recover(), middleware.RequestID(), middleware.AccessLog())
	r.Use(middleware.CORS(o.Cfg.Admin.CORS, o.Cfg.Demo.Enabled))
	r.Use(middleware.IPAllowlist(o.Cfg.Admin.AllowedIPs))

	h := o.Handler

	// 健康检查与指标（不放 /api 下，避免被鉴权挡住）
	r.GET("/healthz", h.Healthz)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

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
			authed.GET("/sites", h.SiteList)
			authed.GET("/settings", h.SettingsGet)
			authed.GET("/settings/versions", h.SettingsVersions)
			authed.GET("/audit", h.AuditList)

			// 规则测试是纯计算，不进写组（演示模式下也要能用）
			authed.POST("/rules/test", h.RuleTest)

			// 写操作：CSRF → DemoGuard → 审计 → 权限
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

	mountSPA(r)
	return r
}

// mountSPA 挂载前端，并处理 history 路由 fallback。
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

func serveIndex(c *gin.Context, sub fs.FS) {
	body, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		c.String(http.StatusNotFound, "index.html 缺失")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", body)
}
