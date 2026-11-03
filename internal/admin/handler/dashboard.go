package handler

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
)

// Overview 汇总指标。
func (s *Server) Overview(c *gin.Context) {
	from, to, label := parseRange(c)
	site := c.Query("site")

	ov, err := s.Store.Overview(site, from, to)
	if err != nil {
		s.failDB(c, err)
		return
	}
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

// Timeseries 时间序列。
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

// Top TopN。
func (s *Server) Top(c *gin.Context) {
	from, to, _ := parseRange(c)
	field := c.DefaultQuery("field", "ip")
	site := c.Query("site")
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

// Categories 攻击类型分布。
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

// SystemInfo 系统信息，前端用它判断是否演示模式。
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

// Healthz 健康检查（不回环也安全，只返回极简信息）。
func (s *Server) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": s.Version})
}

// MetricsDebug 返回关键计数（Prometheus 指标另走 /metrics）。
func (s *Server) MetricsDebug(c *gin.Context) {
	response.OK(c, gin.H{
		"banned":        s.RT.Bans.Size(),
		"eventsWrote":   s.RT.Events.Written(),
		"eventsDropped": s.RT.Events.Dropped(),
		"limiterSize":   s.RT.Limiter.Size(),
	})
}
