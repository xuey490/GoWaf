package handler

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// allowedSettingKeys 允许通过 HTTP 修改的设置键白名单，防止写入任意键污染配置。
// 各键名与取值语义定义见 internal/runtime。
var allowedSettingKeys = map[string]struct{}{
	runtime.KeyMode:           {},
	runtime.KeyBlockScore:     {},
	runtime.KeyChallengeScore: {},
	runtime.KeySampleRate:     {},
	runtime.KeyIPRPS:          {},
	runtime.KeyIPBurst:        {},
	runtime.KeyIPPerMinute:    {},
	runtime.KeyAutoBan:        {},
	runtime.KeyBanThreshold:   {},
	runtime.KeyBanWindow:      {},
	runtime.KeyBanTTL:         {},
	runtime.KeyBanMaxStrikes:  {},
	runtime.KeyBanStrikeReset: {},
	runtime.KeyCRSEnabled:     {},
	runtime.KeyCRSMode:        {},
	runtime.KeyCRSParanoia:    {},

	runtime.KeyIntelBanOnHit: {},
	runtime.KeyIntelBanTTL:   {},

	runtime.KeyNotifyEnabled:  {},
	runtime.KeyNotifyHost:     {},
	runtime.KeyNotifyPort:     {},
	runtime.KeyNotifyUser:     {},
	runtime.KeyNotifyPass:     {},
	runtime.KeyNotifyFrom:     {},
	runtime.KeyNotifyTo:       {},
	runtime.KeyNotifyTLS:      {},
	runtime.KeyNotifySubject:  {},
	runtime.KeyNotifyMinScore: {},
	runtime.KeyNotifyWindow:   {},
	runtime.KeyNotifyCooldown: {},
}

// SettingsGet 返回全部设置与引擎概要。
// 参数 c：Gin 上下文，无查询参数。
// 返回：settings 键值表（SMTP 密码以掩码 ****** 回显）、engine 引擎信息、
// demo 与 banner 演示模式标志。
// 路由：GET /api/settings。
func (s *Server) SettingsGet(c *gin.Context) {
	sm := s.RT.Settings()
	out := make(map[string]string, len(sm))
	for k, v := range sm {
		out[k] = v
	}
	// SMTP 密码不回显明文：面板只显示掩码，保存时留空/保持掩码即不修改。
	if out[runtime.KeyNotifyPass] != "" {
		out[runtime.KeyNotifyPass] = "******"
	}
	// 补上只读展示项
	out[runtime.KeyMode] = s.RT.Engine.Mode()
	response.OK(c, gin.H{
		"settings": out,
		"engine":   s.RT.Engine.Info(),
		"demo":     s.Cfg.Demo.Enabled,
		"banner":   s.Cfg.Demo.Banner,
	})
}

// SettingsUpdate 批量修改设置并触发热加载。
// 参数 c：Gin 上下文，请求体为 key-value 对象，仅白名单内的键可写。
// 返回：全部成功返回 200 与最新设置及当前模式；任一键校验失败或落库失败即整体终止
// （此前已写入的键不回滚）；保存成功但重载失败返回 500 并提示。
// 路由：PUT /api/settings（要求 settings:write 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) SettingsUpdate(c *gin.Context) {
	var in map[string]string
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误，应为 key-value 对象")
		return
	}
	if len(in) == 0 {
		response.BadRequest(c, "没有需要修改的项")
		return
	}

	for k, v := range in {
		if _, ok := allowedSettingKeys[k]; !ok {
			response.BadRequest(c, "不支持的设置项: "+k)
			return
		}
		// 密码框拿到的是掩码，说明用户没动过这一项
		if k == runtime.KeyNotifyPass && v == "******" {
			continue
		}
		if msg := validateSetting(k, v); msg != "" {
			response.BadRequest(c, msg)
			return
		}
		if err := s.touchSettings(k, v, c.GetString(middleware.CtxUserName)); err != nil {
			s.failDB(c, err)
			return
		}
	}

	// 快照，供回滚
	snapshot, _ := json.Marshal(s.RT.Settings())
	s.Store.Gorm().Create(&model.ConfigVersion{
		Snapshot:  string(snapshot),
		Operator:  c.GetString(middleware.CtxUserName),
		Remark:    "settings update",
		CreatedAt: store.NowMS(),
	})

	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "设置已保存但重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "settings")
	response.OK(c, gin.H{"settings": s.RT.Settings(), "mode": s.RT.Engine.Mode()})
}

// validateSetting 校验单个设置项取值的格式与范围，避免把系统调到不可用。
// 参数 key：设置键名；value：待写入的字符串值。
// 返回：空串表示合法；否则返回可直接展示给前端的中文错误信息。
func validateSetting(key, value string) string {
	switch key {
	case runtime.KeyMode:
		if value != "observe" && value != "protect" {
			return "engine.mode 只能是 observe 或 protect"
		}
	case runtime.KeyBlockScore, runtime.KeyChallengeScore:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 || n > 1000 {
			return key + " 必须是 1~1000 的整数"
		}
	case runtime.KeySampleRate:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || f < 0 || f > 1 {
			return "sample_rate 必须是 0~1 之间"
		}
	case runtime.KeyIPRPS, runtime.KeyIPBurst, runtime.KeyIPPerMinute,
		runtime.KeyBanThreshold, runtime.KeyBanWindow, runtime.KeyBanTTL,
		runtime.KeyBanMaxStrikes, runtime.KeyBanStrikeReset:
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return key + " 必须是非负整数"
		}
	case runtime.KeyCRSEnabled:
		if value != "true" && value != "false" {
			return "engine.crs.enabled 只能是 true 或 false"
		}
	case runtime.KeyCRSMode:
		if value != "block" && value != "log" {
			return "engine.crs.mode 只能是 block 或 log"
		}
	case runtime.KeyCRSParanoia:
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 4 {
			return "engine.crs.paranoia 只能是 1~4"
		}
	case runtime.KeyAutoBan:
		if value != "true" && value != "false" {
			return "guard.auto_ban 只能是 true/false"
		}
	case runtime.KeyIntelBanOnHit, runtime.KeyNotifyEnabled:
		if value != "true" && value != "false" {
			return key + " 只能是 true 或 false"
		}
	case runtime.KeyIntelBanTTL:
		n, err := strconv.Atoi(value)
		if err != nil || n < 60 || n > 604800 {
			return "intel.ban_ttl 必须是 60~604800 秒"
		}
	case runtime.KeyNotifyPort:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 || n > 65535 {
			return "notify.email.port 必须是 1~65535"
		}
	case runtime.KeyNotifyTLS:
		if value != "none" && value != "starttls" && value != "ssl" {
			return "notify.email.tls 只能是 none / starttls / ssl"
		}
	case runtime.KeyNotifyMinScore, runtime.KeyNotifyWindow, runtime.KeyNotifyCooldown:
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return key + " 必须是非负整数"
		}
	}
	return ""
}

// SettingsReload 手动触发配置全量重载，把数据库中的最新设置刷进运行时。
// 参数 c：Gin 上下文，无请求体。
// 返回：成功返回 200 与引擎信息、当前模式、已加载站点数；失败返回 500（旧配置继续生效）。
// 路由：POST /api/settings/reload（要求 settings:write 权限）。
func (s *Server) SettingsReload(c *gin.Context) {
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "重载失败，已保留旧配置: "+err.Error())
		return
	}
	response.OK(c, gin.H{
		"engine": s.RT.Engine.Info(),
		"mode":   s.RT.Engine.Mode(),
		"sites":  s.RT.Router().Count(),
	})
}

// SettingsVersions 分页返回配置变更历史（每次保存设置都会生成一份快照）。
// 参数 c：Gin 上下文，查询参数 current/size 由 pageParams 解析。
// 返回：按版本号倒序的分页列表，含操作者、备注与完整快照。
// 路由：GET /api/settings/versions。
func (s *Server) SettingsVersions(c *gin.Context) {
	current, size := pageParams(c)
	var total int64
	if err := s.Store.Gorm().Model(&model.ConfigVersion{}).Count(&total).Error; err != nil {
		s.failDB(c, err)
		return
	}
	var rows []model.ConfigVersion
	if err := s.Store.Gorm().Order("version DESC").
		Offset((current - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		list = append(list, gin.H{
			"version":   r.Version,
			"operator":  r.Operator,
			"remark":    r.Remark,
			"createdAt": r.CreatedAt,
			"snapshot":  r.Snapshot,
		})
	}
	response.OK(c, response.NewPage(list, current, size, total))
}

// SettingsRollback 把设置整体回滚到指定历史版本，再触发一次全量重载。
// 参数 c：Gin 上下文，请求体为 {"version": 目标版本号}。
// 返回：成功返回 200 与回滚目标版本及最新设置；版本不存在返回 404，快照解析失败返回 500。
// 路由：POST /api/settings/rollback（要求 settings:write 权限）。
func (s *Server) SettingsRollback(c *gin.Context) {
	var in struct {
		Version int `json:"version" binding:"required"` // 目标配置版本号，必填
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "version 必填")
		return
	}
	var row model.ConfigVersion
	if err := s.Store.Gorm().Where("version = ?", in.Version).First(&row).Error; err != nil {
		response.NotFound(c, "配置版本不存在")
		return
	}
	var snapshot map[string]string
	if err := json.Unmarshal([]byte(row.Snapshot), &snapshot); err != nil {
		response.Internal(c, "快照解析失败")
		return
	}
	for k, v := range snapshot {
		if _, ok := allowedSettingKeys[k]; !ok {
			continue
		}
		if err := s.touchSettings(k, v, c.GetString(middleware.CtxUserName)); err != nil {
			s.failDB(c, err)
			return
		}
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "回滚后重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "settings")
	response.OK(c, gin.H{"rolledBackTo": in.Version, "settings": s.RT.Settings()})
}

// AuditList 分页查询审计日志，支持按时间段与操作者/动作/对象过滤。
// 参数 c：Gin 上下文，查询参数：current/size（分页）、from/to（毫秒时间戳区间）、
// operator（模糊匹配）、action 与 object（精确匹配）。
// 返回：按 id 倒序的分页列表，含变更前后值（before/after）与执行结果。
// 路由：GET /api/audit。
func (s *Server) AuditList(c *gin.Context) {
	current, size := pageParams(c)
	from, to, _ := parseRange(c)

	q := s.Store.Gorm().Model(&model.AuditLog{}).
		Where("ts >= ? AND ts <= ?", from, to)
	if op := strings.TrimSpace(c.Query("operator")); op != "" {
		q = q.Where("operator LIKE ?", "%"+op+"%")
	}
	if act := c.Query("action"); act != "" {
		q = q.Where("action = ?", act)
	}
	if obj := c.Query("object"); obj != "" {
		q = q.Where("object = ?", obj)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		s.failDB(c, err)
		return
	}
	var rows []model.AuditLog
	if err := q.Order("id DESC").Offset((current - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		list = append(list, gin.H{
			"id": r.ID, "ts": r.TS, "operator": r.Operator, "clientIp": r.ClientIP,
			"action": r.Action, "object": r.Object, "objectId": r.ObjectID,
			"before": r.Before, "after": r.After, "result": r.Result,
		})
	}
	response.OK(c, response.NewPage(list, current, size, total))
}
