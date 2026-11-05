package handler

import (
	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
	"gowaf/internal/store/model"
)

// GeoStatus 返回 IP 归属地库状态：是否就绪、网段条数与几条示例查询结果。
// 路由：GET /api/geo/status（需登录）。
// 参数 c：Gin 上下文（无查询参数）。
// 返回：无（200 返回 ready/ranges/sample）。
func (s *Server) GeoStatus(c *gin.Context) {
	response.OK(c, gin.H{
		"ready":  s.RT.Geo.Ready(),
		"ranges": s.RT.Geo.Size(),
		"sample": gin.H{
			"1.0.1.5":   s.RT.Geo.LookupString("1.0.1.5"),
			"8.8.8.8":   s.RT.Geo.LookupString("8.8.8.8"),
			"127.0.0.1": s.RT.Geo.LookupString("127.0.0.1"),
		},
	})
}

// GeoBackfill 给历史事件补齐 IP 归属地（country 字段）。
// 路由：POST /api/geo/backfill（写操作组：CSRF 校验、演示只读拦截、审计、settings:write 权限）。
// 参数 c：Gin 上下文，查询参数 limit 控制单次处理条数（1~50000，缺省 5000）。
// 返回：无（200 返回 updated/scanned/remaining/done；数据库失败返回 500）。
//
// 升级前写入的事件 country 为空，这里分批回填；limit 控制单次处理量，
// 面板可以多次点击，避免一次性锁库太久。
func (s *Server) GeoBackfill(c *gin.Context) {
	limit := 5000
	if v := c.Query("limit"); v != "" {
		if n, err := atoiSafe(v); err == nil && n > 0 && n <= 50000 {
			limit = n
		}
	}

	var rows []model.Event
	if err := s.Store.Gorm().Select("id, client_ip").
		Where("country = '' OR country IS NULL").Limit(limit).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if len(rows) == 0 {
		response.OK(c, gin.H{"updated": 0, "remaining": 0, "done": true})
		return
	}

	// 逐条查询归属地并回写：库中查不到（空串）或单条更新失败时跳过，不中断整体回填
	updated := 0
	for _, r := range rows {
		region := s.RT.Geo.LookupString(r.ClientIP)
		if region == "" {
			continue
		}
		if err := s.Store.Gorm().Model(&model.Event{}).Where("id = ?", r.ID).
			Update("country", region).Error; err != nil {
			continue
		}
		updated++
	}

	var remaining int64
	s.Store.Gorm().Model(&model.Event{}).Where("country = '' OR country IS NULL").Count(&remaining)
	response.OK(c, gin.H{
		"updated":   updated,
		"scanned":   len(rows),
		"remaining": remaining,
		"done":      remaining == 0,
	})
}

// atoiSafe 把字符串解析为非负整数：自行逐字符解析，拒绝数字以外的字符并限制结果上限。
// 参数 s：待解析的纯数字字符串。
// 返回：解析结果与错误；空串返回 (0, nil)；含非数字字符或结果超过 1<<30 时返回 errInvalidNumber。
func atoiSafe(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, nil
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidNumber
		}
		n = n*10 + int(c-'0')
		if n > 1<<30 {
			return 0, errInvalidNumber
		}
	}
	return n, nil
}

// errInvalidNumber atoiSafe 解析失败时返回的统一错误实例。
var errInvalidNumber = &numError{"invalid number"}

// numError 极简错误类型，仅携带一条固定文案。
type numError struct{ msg string }

// Error 实现 error 接口，返回固定文案。
func (e *numError) Error() string { return e.msg }
