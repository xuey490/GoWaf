package handler

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/detect"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// ruleDTO 规则返回结构（keywords 反序列化成数组，前端可直接编辑）。
type ruleDTO struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Score    int      `json:"score"`
	Targets  []string `json:"targets"`
	Keywords []string `json:"keywords"`
	Pattern  string   `json:"pattern"`
	Action   string   `json:"action"`
	Enabled  int      `json:"enabled"`
	Builtin  int      `json:"builtin"`
	Priority int      `json:"priority"`
	Remark   string   `json:"remark"`
	Source   string   `json:"source"`
	Vendor   string   `json:"vendor"`
	Version  string   `json:"version"`
	Group    string   `json:"group"`
	Mode     string   `json:"mode"`
	Hits     int64    `json:"hits"`
	LastHit  int64    `json:"lastHitAt"`
}

func toRuleDTO(r model.Rule) ruleDTO {
	var kws []string
	_ = json.Unmarshal([]byte(r.Keywords), &kws)
	if kws == nil {
		kws = []string{}
	}
	targets := strings.Split(r.Targets, ",")
	if len(targets) == 0 {
		targets = []string{"path"}
	}
	return ruleDTO{
		ID: r.ID, Name: r.Name, Category: r.Category, Score: r.Score,
		Targets: targets, Keywords: kws, Pattern: r.Pattern, Action: r.Action,
		Enabled: r.Enabled, Builtin: r.Builtin, Priority: r.Priority, Remark: r.Remark,
		Source: r.Source, Vendor: r.Vendor, Version: r.Version, Group: r.Group,
		Mode: r.Mode, Hits: r.Hits, LastHit: r.LastHitAt,
	}
}

// RuleList 规则列表。
func (s *Server) RuleList(c *gin.Context) {
	current, size := pageParams(c)
	category := c.Query("category")
	enabled := c.Query("enabled")

	q := s.Store.Gorm().Model(&model.Rule{})
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if enabled == "0" || enabled == "1" {
		q = q.Where("enabled = ?", enabled)
	}
	// 规则包分组过滤：前端左侧分组面板点击后只显示该组
	if group := strings.TrimSpace(c.Query("group")); group != "" {
		q = q.Where("rule_group = ?", group)
	}
	if source := strings.TrimSpace(c.Query("source")); source != "" {
		q = q.Where("source = ?", source)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("id LIKE ? OR name LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		s.failDB(c, err)
		return
	}
	var rows []model.Rule
	if err := q.Order("priority ASC, id ASC").
		Offset((current - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	list := make([]ruleDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, toRuleDTO(r))
	}
	response.OK(c, response.NewPage(list, current, size, total))
}

// ruleInput 新增/修改入参。
type ruleInput struct {
	ID       string   `json:"id"`
	Name     string   `json:"name" binding:"required"`
	Category string   `json:"category" binding:"required"`
	Score    int      `json:"score"`
	Targets  []string `json:"targets"`
	Keywords []string `json:"keywords"`
	Pattern  string   `json:"pattern"`
	Action   string   `json:"action"`
	Priority int      `json:"priority"`
	Remark   string   `json:"remark"`
	Group    string   `json:"group"`
	Mode     string   `json:"mode"`
}

// validateRule 编译校验，避免把编译不过的规则写进库导致全量重载失败。
func validateRule(in *ruleInput) string {
	if in.Pattern != "" {
		pattern := in.Pattern
		if !strings.HasPrefix(pattern, "(?i)") {
			pattern = "(?i)" + pattern
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return "正则编译失败: " + err.Error()
		}
	}
	if in.Pattern == "" && len(in.Keywords) == 0 {
		return "关键词与正则至少填一个"
	}
	if in.Action != "" && in.Action != "block" && in.Action != "challenge" && in.Action != "log" && in.Action != "ban" {
		return "action 只能是 block/challenge/log/ban"
	}
	if in.Mode != "" && in.Mode != "inherit" && in.Mode != "log" && in.Mode != "block" {
		return "mode 只能是 inherit/log/block"
	}
	for _, t := range in.Targets {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "path", "query", "body", "header", "cookie", "ua":
		default:
			return "未知检测目标: " + t
		}
	}
	return ""
}

// RuleCreate 新增自定义规则。
func (s *Server) RuleCreate(c *gin.Context) {
	var in ruleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "名称与分类必填")
		return
	}
	if in.ID == "" {
		in.ID = "custom-" + strconv.FormatInt(store.NowMS(), 36)
	}
	if in.Score <= 0 {
		in.Score = 40
	}
	if in.Action == "" {
		in.Action = "block"
	}
	if in.Priority == 0 {
		in.Priority = 100
	}
	if len(in.Targets) == 0 {
		in.Targets = []string{"path", "query", "body"}
	}
	if msg := validateRule(&in); msg != "" {
		response.BadRequest(c, msg)
		return
	}

	kws, _ := json.Marshal(in.Keywords)
	group := in.Group
	if group == "" {
		group = "custom"
	}
	mode := in.Mode
	if mode == "" {
		mode = "inherit"
	}
	row := model.Rule{
		ID: in.ID, Name: in.Name, Category: in.Category, Score: in.Score,
		Targets: strings.Join(in.Targets, ","), Keywords: string(kws), Pattern: in.Pattern,
		Action: in.Action, Enabled: 1, Builtin: 0, Priority: in.Priority,
		Remark: in.Remark, UpdatedAt: store.NowMS(),
		Source: "custom", Vendor: "gowaf", Version: "1", Group: group, Mode: mode,
	}
	if err := s.Store.Gorm().Create(&row).Error; err != nil {
		response.Fail(c, 409, response.CodeConflict, "规则 ID 已存在")
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已保存但规则重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "rule")
	c.Set(middleware.CtxAuditID, row.ID)
	response.OK(c, toRuleDTO(row))
}

// RuleUpdate 修改规则。内置规则允许改分值/开关，但不允许改 ID。
func (s *Server) RuleUpdate(c *gin.Context) {
	id := c.Param("id")
	var row model.Rule
	if err := s.Store.Gorm().Where("id = ?", id).First(&row).Error; err != nil {
		response.NotFound(c, "规则不存在")
		return
	}

	var in ruleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}

	row.Name = in.Name
	row.Category = in.Category
	if in.Score > 0 {
		row.Score = in.Score
	}
	if len(in.Targets) > 0 {
		row.Targets = strings.Join(in.Targets, ",")
	}
	kws, _ := json.Marshal(in.Keywords)
	row.Keywords = string(kws)
	row.Pattern = in.Pattern
	if in.Action != "" {
		row.Action = in.Action
	}
	if in.Priority > 0 {
		row.Priority = in.Priority
	}
	if in.Mode != "" {
		row.Mode = in.Mode
	}
	if in.Group != "" {
		row.Group = in.Group
	}
	row.Remark = in.Remark
	row.UpdatedAt = store.NowMS()

	if msg := validateRule(&ruleInput{
		Name: row.Name, Category: row.Category, Pattern: row.Pattern,
		Keywords: in.Keywords, Targets: strings.Split(row.Targets, ","), Action: row.Action,
	}); msg != "" {
		response.BadRequest(c, msg)
		return
	}

	if err := s.Store.Gorm().Save(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已保存但规则重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "rule")
	c.Set(middleware.CtxAuditID, id)
	response.OK(c, toRuleDTO(row))
}

// RuleDelete 删除自定义规则；内置规则只能停用。
func (s *Server) RuleDelete(c *gin.Context) {
	id := c.Param("id")
	var row model.Rule
	if err := s.Store.Gorm().Where("id = ?", id).First(&row).Error; err != nil {
		response.NotFound(c, "规则不存在")
		return
	}
	if row.Builtin == 1 {
		response.BadRequest(c, "内置规则不可删除，请改用停用")
		return
	}
	if err := s.Store.Gorm().Delete(&model.Rule{}, "id = ?", id).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已删除但规则重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "rule")
	c.Set(middleware.CtxAuditID, id)
	response.OK(c, gin.H{"ok": true})
}

// RuleToggle 启用/停用。
func (s *Server) RuleToggle(c *gin.Context) {
	id := c.Param("id")
	var row model.Rule
	if err := s.Store.Gorm().Where("id = ?", id).First(&row).Error; err != nil {
		response.NotFound(c, "规则不存在")
		return
	}
	next := 0
	if row.Enabled == 0 {
		next = 1
	}
	if err := s.Store.Gorm().Model(&model.Rule{}).Where("id = ?", id).
		Updates(map[string]any{"enabled": next, "updated_at": store.NowMS()}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "已切换但规则重载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "rule")
	c.Set(middleware.CtxAuditID, id)
	response.OK(c, gin.H{"id": id, "enabled": next})
}

// RuleTest 规则测试器：不转发、不落库。
func (s *Server) RuleTest(c *gin.Context) {
	var in detect.TestConfig
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "method 与 path 必填")
		return
	}
	if !strings.HasPrefix(in.Path, "/") {
		in.Path = "/" + in.Path
	}
	result := s.RT.Engine.Test(in)
	response.OK(c, result)
}

// RuleReload 重新编译规则并返回统计。
func (s *Server) RuleReload(c *gin.Context) {
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "重载失败，已保留旧规则: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "rule")
	response.OK(c, s.RT.Engine.Info())
}
