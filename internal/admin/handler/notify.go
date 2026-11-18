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

// NotifyTest 用已保存的配置发一条测试告警。
// 参数 c：Gin 上下文，请求体可选 {"to": "收件人列表", "channel": "email|webhook"}。
// channel=webhook 时走 webhook 渠道测试（逐目标回显结果），否则测邮件。
// 返回：邮件成功返回 200 与 {"ok":true,"to":[...]}；webhook 返回 {"ok":bool,"results":[...]}；
// 未启用对应渠道、未填收件人或发送失败返回 400。无论成败都会写入 notify_logs。
// 路由：POST /api/notify/test（要求 settings:write 权限）。
//
// 设计上不接收完整的 SMTP/webhook 参数：密码与签名密钥在库里是掩码，前端拿不到
// 明文，传上来再校验会绕一圈还可能把明文写进日志。要求先保存再测试，
// 语义也更清楚——测的就是"当前生效的配置"。
func (s *Server) NotifyTest(c *gin.Context) {
	var in struct {
		To      string `json:"to"`      // 临时收件人（仅邮件渠道），逗号/分号/空白分隔
		Channel string `json:"channel"` // 渠道：email（默认）/ webhook
	}
	_ = c.ShouldBindJSON(&in)

	if strings.TrimSpace(in.Channel) == "webhook" {
		wcfg := s.RT.NotifyConfig()
		if !wcfg.Webhook.Enabled {
			response.BadRequest(c, "请先勾选「启用 Webhook 渠道」并保存，再测试发送")
			return
		}
		results := notify.SendTestWebhook(wcfg)
		if len(results) == 0 {
			response.BadRequest(c, "尚未配置任何 webhook 地址（飞书/钉钉/企业微信/通用）")
			return
		}
		allOK := true
		for _, r := range results {
			if !r.OK {
				allOK = false
			}
		}
		if !allOK {
			// 用 400 而不是 500：机器人地址错误/签名错误是配置问题，不是服务端故障。
			response.Fail(c, 400, response.CodeBadRequest, "部分 webhook 发送失败，详见 results")
			return
		}
		response.OK(c, gin.H{"ok": allOK, "results": results})
		return
	}

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
	if strings.TrimSpace(in.To) != "" {
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
		// 用 400 而不是 500：SMTP 认证/连接失败是配置问题，不是服务端故障。
		// 前端拦截器只对 5xx 做自动重试，用 500 会导致重复投递。
		response.BadRequest(c, "发送失败: "+err.Error())
		return
	}
	_ = s.Store.Gorm().Create(&lg).Error
	response.OK(c, gin.H{"ok": true, "to": cfg.To})
}

// NotifyLogs 返回最近的告警发送记录与当前生效的通道摘要，用于排障。
// 参数 c：Gin 上下文，查询参数 size（返回条数，默认 20，上限 200）。
// 返回：list 按 id 倒序的发送记录；channel 邮件通道摘要（时间字段已换算为秒）；
// intelBan 情报命中自动封禁的开关与 TTL。
// 路由：GET /api/notify/logs。
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
			// webhook 渠道摘要：前端在渠道列表里渲染各目标配置状态
			"webhook": gin.H{
				"enabled":  cfg.Webhook.Enabled,
				"feishu":   cfg.Webhook.FeishuURL != "",
				"dingtalk": cfg.Webhook.DingURL != "",
				"wecom":    cfg.Webhook.WecomURL != "",
				"generic":  cfg.Webhook.GenericURL != "",
				"window":   int(cfg.Window / time.Second),
				"cooldown": int(cfg.Cooldown / time.Second),
			},
		},
		"intelBan": gin.H{
			"enabled": s.RT.SettingBool(runtime.KeyIntelBanOnHit, true),
			"ttl":     s.RT.SettingInt(runtime.KeyIntelBanTTL, 3600),
		},
	})
}
