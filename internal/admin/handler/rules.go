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
	ID       string   `json:"id"`        // 规则唯一标识
	Name     string   `json:"name"`      // 规则名称
	Category string   `json:"category"`  // 分类（如 sqli、xss）
	Score    int      `json:"score"`     // 命中时累加的分值
	Targets  []string `json:"targets"`   // 检测目标：path/query/body/header/cookie/ua
	Keywords []string `json:"keywords"`  // 关键词列表（AC 预筛，任一命中才继续跑正则）
	Pattern  string   `json:"pattern"`   // 正则表达式（可为空，表示仅按关键词匹配）
	Action   string   `json:"action"`    // 处置动作：block/challenge/log/ban
	Enabled  int      `json:"enabled"`   // 启用状态：1 启用 / 0 停用
	Builtin  int      `json:"builtin"`   // 是否内置规则：1 内置（不可删除）/ 0 自定义
	Priority int      `json:"priority"`  // 优先级，数字越小越先执行
	Remark   string   `json:"remark"`    // 备注说明
	Source   string   `json:"source"`    // 来源：builtin/custom/imported
	Vendor   string   `json:"vendor"`    // 维护方（规则包厂商）
	Version  string   `json:"version"`   // 规则包版本号
	Group    string   `json:"group"`     // 规则包分组名
	Mode     string   `json:"mode"`      // 检测模式：inherit（跟随引擎）/log（只记录不计分）/block
	Hits     int64    `json:"hits"`      // 累计命中次数（由后台任务定期回写）
	LastHit  int64    `json:"lastHitAt"` // 最近命中时间（毫秒时间戳，0 表示从未命中）
}

// toRuleDTO 把规则模型记录转换为前端 DTO。
// 参数 r：规则模型记录，其 Keywords 为 JSON 数组字符串、Targets 为逗号分隔字符串。
// 返回：ruleDTO；Keywords 解析失败时降级为空数组，Targets 按逗号拆分为数组。
func toRuleDTO(r model.Rule) ruleDTO {
	var kws []string
	_ = json.Unmarshal([]byte(r.Keywords), &kws) // 解析失败不阻断，按空数组处理
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

// RuleList 分页返回规则列表，支持按分类、状态、分组、来源与关键字过滤。
// 路由：GET /api/rules（需登录）。
// 参数 c：Gin 上下文；查询参数 current/size 分页，category、enabled（0/1）、
// group（规则包分组）、source（来源）、keyword（按 ID 或名称模糊匹配）可选。
// 返回：无（200 返回分页包装的 ruleDTO 列表，数据库错误返回 500）。
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
	ID       string   `json:"id"`                          // 规则 ID；新增时留空则自动生成 custom- 前缀 ID
	Name     string   `json:"name" binding:"required"`     // 规则名称（必填）
	Category string   `json:"category" binding:"required"` // 分类（必填）
	Score    int      `json:"score"`                       // 命中分值；新增时 ≤0 默认 40
	Targets  []string `json:"targets"`                     // 检测目标列表；新增时为空默认 path,query,body
	Keywords []string `json:"keywords"`                    // 关键词列表（与 pattern 至少填一个）
	Pattern  string   `json:"pattern"`                     // 正则表达式（与 keywords 至少填一个）
	Action   string   `json:"action"`                      // 处置动作；新增时为空默认 block
	Priority int      `json:"priority"`                    // 优先级；新增时为 0 默认 100
	Remark   string   `json:"remark"`                      // 备注说明
	Group    string   `json:"group"`                       // 规则包分组；新增时为空归入 custom
	Mode     string   `json:"mode"`                        // 检测模式；新增时为空默认 inherit
}

// validateRule 在落库前编译校验规则参数，避免把编译不过的规则写进库导致全量重载失败。
// 参数 in：待校验的规则入参（正则、关键词、动作、模式、检测目标）。
// 返回：空字符串表示校验通过；否则返回可直接展示给用户的中文错误文案。
func validateRule(in *ruleInput) string {
	if in.Pattern != "" {
		pattern := in.Pattern
		if !strings.HasPrefix(pattern, "(?i)") {
			pattern = "(?i)" + pattern // 与检测引擎行为一致，默认大小写不敏感
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
		case "path", "query", "body", "header", "cookie", "ua": // 合法检测目标
		default:
			return "未知检测目标: " + t
		}
	}
	return ""
}

// RuleCreate 新增自定义规则。
// 路由：POST /api/rules（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，JSON 正文为 ruleInput。
// 返回：无（200 返回新规则的 ruleDTO；参数非法 400、规则 ID 冲突 409、
// 规则重载失败 500 但数据已落库）。
func (s *Server) RuleCreate(c *gin.Context) {
	var in ruleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "名称与分类必填")
		return
	}
	// 以下为各字段的缺省值兜底
	if in.ID == "" {
		in.ID = "custom-" + strconv.FormatInt(store.NowMS(), 36) // 毫秒时间戳转 36 进制，保证不重复
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

	kws, _ := json.Marshal(in.Keywords) // 关键词序列化为 JSON 数组落库
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
	c.Set(middleware.CtxAuditObj, "rule") // 审计对象类型，供 Audit 中间件落库
	c.Set(middleware.CtxAuditID, row.ID)  // 审计对象 ID
	response.OK(c, toRuleDTO(row))
}

// RuleUpdate 修改规则。规则 ID 不可改（入参 ID 被忽略），其余字段按入参合并更新。
// 路由：PUT /api/rules/:id（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，路径参数 id 为规则 ID，JSON 正文为 ruleInput。
// 返回：无（200 返回更新后的 ruleDTO；规则不存在 404、参数非法或校验失败 400、
// 重载失败 500 但数据已落库）。
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
	// 零值字段表示「保持原值」，避免更新时把已有配置误清掉
	if in.Score > 0 {
		row.Score = in.Score
	}
	if len(in.Targets) > 0 {
		row.Targets = strings.Join(in.Targets, ",")
	}
	kws, _ := json.Marshal(in.Keywords)
	row.Keywords = string(kws) // 关键词全量覆盖（传空数组即清空）
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

	// 用合并后的最终字段再校验一次，防止更新后出现编译不过的规则
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

// RuleDelete 删除自定义规则；内置规则只能停用，不允许删除。
// 路由：DELETE /api/rules/:id（写操作组，需 rule:delete 权限）。
// 参数 c：Gin 上下文，路径参数 id 为规则 ID。
// 返回：无（200 返回 {"ok":true}；规则不存在 404、内置规则 400、重载失败 500 但记录已删除）。
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

// RuleToggle 启用/停用规则（在当前状态上取反）。
// 路由：PATCH /api/rules/:id/toggle（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，路径参数 id 为规则 ID。
// 返回：无（200 返回 {"id":规则ID,"enabled":切换后的状态}；规则不存在 404、
// 重载失败 500 但状态已切换）。
func (s *Server) RuleToggle(c *gin.Context) {
	id := c.Param("id")
	var row model.Rule
	if err := s.Store.Gorm().Where("id = ?", id).First(&row).Error; err != nil {
		response.NotFound(c, "规则不存在")
		return
	}
	next := 0
	if row.Enabled == 0 {
		next = 1 // 当前是停用则切换为启用
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

// RuleTest 规则测试器：用模拟请求跑一遍检测引擎，不转发请求、不落库。
// 路由：POST /api/rules/test（仅需登录；纯计算，演示模式下也可用）。
// 参数 c：Gin 上下文，JSON 正文为 detect.TestConfig（method 与 path 必填）。
// 返回：无（200 返回命中的规则、分类与分值等检测结果；参数非法 400）。
func (s *Server) RuleTest(c *gin.Context) {
	var in detect.TestConfig
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "method 与 path 必填")
		return
	}
	if !strings.HasPrefix(in.Path, "/") {
		in.Path = "/" + in.Path // 容错：允许省略开头的斜杠
	}
	result := s.RT.Engine.Test(in)
	response.OK(c, result)
}

// RuleReload 重新编译全部规则并热加载，返回编译后的规则集统计。
// 路由：POST /api/rules/reload（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文。
// 返回：无（200 返回规则集统计（条数/关键词数/版本）；编译失败返回 500，
// 检测引擎保留旧规则继续生效）。
func (s *Server) RuleReload(c *gin.Context) {
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "重载失败，已保留旧规则: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "rule")
	response.OK(c, s.RT.Engine.Info())
}
