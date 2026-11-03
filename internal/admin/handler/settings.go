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

// allowedSettingKeys 白名单，防止写入任意键污染配置。
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
}

// SettingsGet 返回全部设置。
func (s *Server) SettingsGet(c *gin.Context) {
	sm := s.RT.Settings()
	out := make(map[string]string, len(sm))
	for k, v := range sm {
		out[k] = v
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

// SettingsUpdate 修改设置并热加载。
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

// validateSetting 关键项做范围校验，避免把系统调到不可用。
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
	case runtime.KeyAutoBan:
		if value != "true" && value != "false" {
			return "guard.auto_ban 只能是 true/false"
		}
	}
	return ""
}

// SettingsReload 手动触发全量重载。
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

// SettingsVersions 配置历史。
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

// SettingsRollback 回滚到指定版本。
func (s *Server) SettingsRollback(c *gin.Context) {
	var in struct {
		Version int `json:"version" binding:"required"`
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

// AuditList 审计日志。
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
