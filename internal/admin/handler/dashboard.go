package handler

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
)

// Overview 返回仪表盘汇总指标：请求总量、放行/拦截/人机挑战数、5xx 数、QPS 与拦截率。
// 路由：GET /api/dashboard/overview（需登录）。
// 参数 c：Gin 上下文，查询参数 range/from/to 控制时间范围，site 过滤站点。
// 返回：无（200 返回指标 JSON；数据库失败返回 500）。
func (s *Server) Overview(c *gin.Context) {
	from, to, label := parseRange(c)
	site := c.Query("site")

	ov, err := s.Store.Overview(site, from, to)
	if err != nil {
		s.failDB(c, err)
		return
	}
	// 拦截率 = 拦截数 / 请求总数；总数为 0 时取 0，避免除零
	blockRate := 0.0
	if ov.Total > 0 {
		blockRate = float64(ov.Blocked) / float64(ov.Total)
	}
	response.OK(c, gin.H{
		"range":      label,
		"from":       from,
		"to":         to,
		"total":      ov.Total,
		"passed":     ov.Passed,
		"blocked":    ov.Blocked,
		"challenged": ov.Challenged,
		"status5xx":  ov.Status5xx,
		"qps":        ov.QPS,
		"blockRate":  blockRate,
		"mode":       s.RT.Engine.Mode(),
	})
}

// Timeseries 返回时间序列统计点，聚合桶宽度按时间跨度自动选择。
// 路由：GET /api/dashboard/timeseries（需登录）。
// 参数 c：Gin 上下文，查询参数 range/from/to 控制时间范围，site 过滤站点。
// 返回：无（200 返回 range/bucket/points；数据库失败返回 500）。
func (s *Server) Timeseries(c *gin.Context) {
	from, to, label := parseRange(c)
	site := c.Query("site")
	bucket := bucketSeconds(from, to)

	points, err := s.Store.Timeseries(site, from, to, bucket)
	if err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{
		"range":  label,
		"bucket": bucket,
		"points": points,
	})
}

// Top 返回指定维度的 TopN 统计（默认按 IP 维度）。
// 路由：GET /api/dashboard/top（需登录）。
// 参数 c：Gin 上下文，查询参数 field 指定统计维度、limit 指定条数、range/from/to 控制时间范围、site 过滤站点。
// 返回：无（200 返回 field 与 items；数据库失败返回 500）。
func (s *Server) Top(c *gin.Context) {
	from, to, _ := parseRange(c)
	field := c.DefaultQuery("field", "ip")
	site := c.Query("site")
	// limit 夹在 1~100 之间，缺省或非法时取 10
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	items, err := s.Store.Top(field, site, from, to, limit)
	if err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"field": field, "items": items})
}

// Categories 返回时间范围内各攻击类型的分布统计。
// 路由：GET /api/dashboard/categories（需登录）。
// 参数 c：Gin 上下文，查询参数 range/from/to 控制时间范围，site 过滤站点。
// 返回：无（200 返回 items；数据库失败返回 500）。
func (s *Server) Categories(c *gin.Context) {
	from, to, _ := parseRange(c)
	site := c.Query("site")

	items, err := s.Store.CategoryStat(site, from, to)
	if err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// SystemInfo 返回系统运行信息：版本、运行时长、内存/Goroutine、规则与站点数量、演示模式开关等。
// 路由：GET /api/system/info（需登录）。
// 参数 c：Gin 上下文（无查询参数）。
// 返回：无（200 返回信息 JSON）。
func (s *Server) SystemInfo(c *gin.Context) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := int64(time.Since(s.RT.StartedAt()).Seconds())
	white, black := s.RT.IPList().Count()
	info := s.RT.Engine.Info()

	resp := gin.H{
		"version":      s.Version,
		"startedAt":    s.RT.StartedAt().UnixMilli(),
		"uptimeSec":    uptime,
		"goroutines":   runtime.NumGoroutine(),
		"memoryMB":     mem.Alloc / 1024 / 1024,
		"heapMB":       mem.HeapAlloc / 1024 / 1024,
		"goVersion":    runtime.Version(),
		"mode":         s.RT.Engine.Mode(),
		"rules":        info.Rules,
		"keywords":     info.Keywords,
		"sites":        s.RT.Router().Count(),
		"bannedIPs":    s.RT.Bans.Size(),
		"whitelist":    white,
		"blacklist":    black,
		"limiterSize":  s.RT.Limiter.Size(),
		"demo":         s.Cfg.Demo.Enabled,
		"demoReadOnly": s.Cfg.Demo.Enabled && s.Cfg.Demo.ReadOnly,
		"demoBanner":   s.Cfg.Demo.Banner,
		// 站点标记的 CDN 与 trusted_proxies 不一致时的风险提示，前端顶部横幅消费
		"cdnWarnings": s.RT.CDNWarnings(),
	}

	if s.Cfg.Demo.Enabled {
		resp["banner"] = s.Cfg.Demo.Banner
	}
	response.OK(c, resp)
}

// Healthz 健康检查端点，只返回状态与版本等极简信息，不鉴权也不会泄露敏感数据。
// 路由：GET /healthz（公开，挂在 /api 鉴权分组之外）。
// 参数 c：Gin 上下文。
// 返回：无（200 返回 {"status":"ok","version":"..."}）。
func (s *Server) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": s.Version})
}

// MetricsDebug 返回关键运行计数（封禁数、事件写入/丢弃数、限流器大小），便于面板排障。
// 路由：GET /api/system/debug（需登录）。完整 Prometheus 指标另走 /metrics。
// 参数 c：Gin 上下文。
// 返回：无（200 返回计数字典）。
func (s *Server) MetricsDebug(c *gin.Context) {
	response.OK(c, gin.H{
		"banned":        s.RT.Bans.Size(),
		"eventsWrote":   s.RT.Events.Written(),
		"eventsDropped": s.RT.Events.Dropped(),
		"limiterSize":   s.RT.Limiter.Size(),
	})
}
