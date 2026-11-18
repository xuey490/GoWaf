package handler

import (
	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/tamper"
)

/* ================================================== 网页防篡改（基线管理） */

// TamperStatus 返回防篡改的设置摘要与基线列表。
// 参数 c：Gin 上下文，无查询参数。
// 返回：enabled/mode/paths/maxBytes 设置摘要 + items 基线列表（站点、路径、
// 哈希、大小、生成时间、是否有副本）。
// 路由：GET /api/tamper/status。
func (s *Server) TamperStatus(c *gin.Context) {
	st := s.RT.TamperSettings()
	response.OK(c, gin.H{
		"enabled":  st.Enabled,
		"mode":     st.Mode,
		"paths":    st.Paths,
		"maxBytes": st.MaxBytes,
		"items":    s.RT.Tamper.List(),
	})
}

// TamperRefresh 重新生成防篡改基线：对每个启用站点 × 每个受保护路径回源抓取。
// 参数 c：Gin 上下文，无请求体。
// 返回：items 逐项抓取结果（成功带哈希与大小；失败带原因并保留旧基线）；
// 防篡改未启用或未配置路径返回 400。
// 路由：POST /api/tamper/refresh（要求 settings:write 权限）。
func (s *Server) TamperRefresh(c *gin.Context) {
	results, err := s.RT.TamperRefresh()
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "tamper")
	okCount := 0
	for _, r := range results {
		if r.OK {
			okCount++
		}
	}
	response.OK(c, gin.H{
		"items":     results,
		"total":     len(results),
		"okCount":   okCount,
		"baselines": s.RT.Tamper.Count(),
	})
}

// tamper 包引用占位（设置校验使用其常量；此文件保持 handler 侧的类型自洽）。
var _ = tamper.ModeObserve
