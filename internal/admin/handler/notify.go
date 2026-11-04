package handler

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
	"gowaf/internal/notify"
	"gowaf/internal/runtime"
	"gowaf/internal/store/model"
)

/* ================================================== 告警通知 */

// NotifyTest 用已保存的配置发一封测试邮件。
//
// 设计上不接收完整的 SMTP 参数：密码在库里是掩码，前端拿不到明文，
// 传上来再校验会绕一圈还可能把明文写进日志。要求先保存再测试，
// 语义也更清楚——测的就是"当前生效的配置"。
func (s *Server) NotifyTest(c *gin.Context) {
	cfg := s.RT.NotifyConfig()
	if !cfg.Enabled {
		response.BadRequest(c, "请先勾选「启用告警通知」并保存，再测试发送")
		return
	}
	if len(cfg.To) == 0 {
		response.BadRequest(c, "请先填写收件人")
		return
	}

	// 允许临时指定收件人，方便"只发给我自己看看"
	var in struct {
		To string `json:"to"`
	}
	if err := c.ShouldBindJSON(&in); err == nil && strings.TrimSpace(in.To) != "" {
		cfg.To = strings.FieldsFunc(in.To, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\n'
		})
	}

	err := notify.SendTest(cfg)
	lg := model.NotifyLog{
		Channel: "email", Target: strings.Join(cfg.To, ","),
		Subject: "测试邮件", Summary: "控制台测试发送",
		Count: 0, Status: "ok", CreatedAt: time.Now().UnixMilli(),
	}
	if err != nil {
		lg.Status, lg.Error = "fail", err.Error()
		_ = s.Store.Gorm().Create(&lg).Error
		response.Internal(c, "发送失败: "+err.Error())
		return
	}
	_ = s.Store.Gorm().Create(&lg).Error
	response.OK(c, gin.H{"ok": true, "to": cfg.To})
}

// NotifyLogs 最近的发送记录，用于排障。
func (s *Server) NotifyLogs(c *gin.Context) {
	size := 20
	if v := c.Query("size"); v != "" {
		if n, err := atoiSafe(v); err == nil && n > 0 && n <= 200 {
			size = n
		}
	}
	var rows []model.NotifyLog
	if err := s.Store.Gorm().Order("id DESC").Limit(size).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	// 顺带把当前生效的通道摘要给前端，省一次请求
	cfg := s.RT.NotifyConfig()
	response.OK(c, gin.H{
		"list": rows,
		"channel": gin.H{
			"enabled":  cfg.Enabled,
			"host":     cfg.Host,
			"port":     cfg.Port,
			"tls":      cfg.TLS,
			"to":       cfg.To,
			"window":   int(cfg.Window / time.Second),
			"cooldown": int(cfg.Cooldown / time.Second),
		},
		"intelBan": gin.H{
			"enabled": s.RT.SettingBool(runtime.KeyIntelBanOnHit, true),
			"ttl":     s.RT.SettingInt(runtime.KeyIntelBanTTL, 3600),
		},
	})
}
