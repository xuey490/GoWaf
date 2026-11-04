package handler

import (
	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
	"gowaf/internal/store/model"
)

// GeoStatus 返回 IP 归属地库状态。
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

// GeoBackfill 给历史事件补齐归属地。
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

var errInvalidNumber = &numError{"invalid number"}

type numError struct{ msg string }

func (e *numError) Error() string { return e.msg }
