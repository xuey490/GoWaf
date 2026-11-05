package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
	"gowaf/internal/detect"
)

/* ============================================================ CRS 上游版本检查 */

// CRSCheckUpdate 检查内嵌 CRS 规则集是否有上游新版本。
//
// 只读出网检查，数据源按 GitHub release → Go 模块代理（proxy.golang.org →
// goproxy.cn）顺序回退；结果服务端缓存 10 分钟，?force=1 忽略缓存强制重查。
// 命中缓存的响应 cached=true，前端据此区分「刚查过」与「真实出网结果」。
// 路由：GET /api/crs/check-update（需登录；只读接口不挂按钮级权限与 CSRF）。
// 参数 c：Gin 上下文，查询参数 force 可选。
// 返回：无（200 返回 detect.CRSUpdateInfo；全部数据源不可达返回 502）。
func (s *Server) CRSCheckUpdate(c *gin.Context) {
	force := c.Query("force") == "1" || c.Query("force") == "true"
	info, err := detect.CheckCRSUpdate(c.Request.Context(), force)
	if err != nil {
		// 上游不可达不是本服务内部错误，用 502 语义；消息里带各数据源失败原因
		response.Fail(c, http.StatusBadGateway, http.StatusBadGateway,
			"检查 CRS 上游版本失败: "+err.Error())
		return
	}
	response.OK(c, info)
}
