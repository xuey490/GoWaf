package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

/* ====================================================== P4 规则例外 */

// ExceptionList 查询规则例外列表。
// 参数 c：Gin 上下文，可选 site（查指定站点的例外，同时包含全局例外）、keyword（对规则 ID、路径前缀、备注模糊匹配）。
// 返回：无（200 返回 {"list": 例外列表, "total": 数量}，最多 500 条；数据库失败返回 500）。
// 路由：GET /api/exceptions（仅需登录）。
func (s *Server) ExceptionList(c *gin.Context) {
	q := s.Store.Gorm().Model(&model.RuleException{})
	if site := strings.TrimSpace(c.Query("site")); site != "" {
		// 指定站点时同时返回全局（site 为空）例外
		q = q.Where("site = ? OR site = ''", site)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("rule_id LIKE ? OR path_prefix LIKE ? OR note LIKE ?",
			"%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
	}
	var rows []model.RuleException
	if err := q.Order("id DESC").Limit(500).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// exceptionInput 新增/修改规则例外的入参，各限定维度留空表示不限。
type exceptionInput struct {
	Site       string `json:"site"`       // 限定站点名，留空表示对所有站点生效
	RuleID     string `json:"ruleId"`     // 限定规则 ID，留空表示不限规则
	Category   string `json:"category"`   // 限定攻击分类，留空表示不限分类
	PathPrefix string `json:"pathPrefix"` // 限定请求路径前缀，留空表示不限路径
	Param      string `json:"param"`      // 限定参数名，留空表示不限参数
	Note       string `json:"note"`       // 备注说明
	Enabled    *int   `json:"enabled"`    // 是否启用（1 启用 / 0 停用）；用指针区分「未传」与传 0，未传时保持默认/原值
}

// validateException 校验例外入参的底线条件。
// 参数 in：待校验的例外入参。
// 返回：错误提示文案；站点/规则 ID/路径前缀/参数名四项全空时返回错误（等于全局关闭检测），否则返回空串表示通过。
func validateException(in *exceptionInput) string {
	if in.Site == "" && in.RuleID == "" && in.PathPrefix == "" && in.Param == "" {
		return "站点/规则/路径/参数至少要填一项，否则等于全局关闭检测"
	}
	return ""
}

// ExceptionCreate 新增规则例外，通常从攻击事件的「误报」按钮一键生成。
// 参数 c：Gin 上下文，JSON 体为 exceptionInput，各文本字段自动去除首尾空白。
// 返回：无（200 返回新建的例外记录；参数非法返回 400，数据库失败返回 500；保存后触发配置热重载，失败仅提示不影响已落库数据）。
// 路由：POST /api/exceptions（需 rule:write 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) ExceptionCreate(c *gin.Context) {
	var in exceptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	in.Site = strings.TrimSpace(in.Site)
	in.RuleID = strings.TrimSpace(in.RuleID)
	in.PathPrefix = strings.TrimSpace(in.PathPrefix)
	in.Param = strings.TrimSpace(in.Param)
	if msg := validateException(&in); msg != "" {
		response.BadRequest(c, msg)
		return
	}
	enabled := 1 // Enabled 未传时默认启用
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	row := model.RuleException{
		Site: in.Site, RuleID: in.RuleID, Category: in.Category,
		PathPrefix: in.PathPrefix, Param: in.Param, Note: in.Note,
		Enabled: enabled, CreatedBy: c.GetString(middleware.CtxUserName),
		CreatedAt: store.NowMS(),
	}
	if err := s.Store.Gorm().Create(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "rule_exception")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(uint64(row.ID), 10))
	response.OK(c, row)
}

// ExceptionUpdate 全量修改规则例外。
// 参数 c：Gin 上下文，路径参数 id 为例外 ID，JSON 体为 exceptionInput（Enabled 未传时保持原值）。
// 返回：无（200 返回更新后的例外记录；id 非法或参数非法返回 400，例外不存在返回 404，数据库失败返回 500；保存后触发配置热重载，失败仅提示不影响已落库数据）。
// 路由：PUT /api/exceptions/:id（需 rule:write 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) ExceptionUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var row model.RuleException
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		response.NotFound(c, "例外不存在")
		return
	}
	var in exceptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	in.Site = strings.TrimSpace(in.Site)
	in.RuleID = strings.TrimSpace(in.RuleID)
	in.PathPrefix = strings.TrimSpace(in.PathPrefix)
	in.Param = strings.TrimSpace(in.Param)
	if msg := validateException(&in); msg != "" {
		response.BadRequest(c, msg)
		return
	}
	row.Site, row.RuleID, row.Category = in.Site, in.RuleID, in.Category
	row.PathPrefix, row.Param, row.Note = in.PathPrefix, in.Param, in.Note
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if err := s.Store.Gorm().Save(&row).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "rule_exception")
	c.Set(middleware.CtxAuditID, c.Param("id"))
	response.OK(c, row)
}

// ExceptionToggle 按当前状态取反，启用/停用规则例外。
// 参数 c：Gin 上下文，路径参数 id 为例外 ID，无请求体。
// 返回：无（200 返回 {"id": 例外 ID, "enabled": 切换后的状态}；id 非法返回 400，例外不存在返回 404，数据库失败返回 500；保存后触发配置热重载，失败仅提示不影响已落库数据）。
// 路由：PATCH /api/exceptions/:id/toggle（需 rule:write 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) ExceptionToggle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	var row model.RuleException
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		response.NotFound(c, "例外不存在")
		return
	}
	next := 0
	if row.Enabled == 0 {
		next = 1
	}
	if err := s.Store.Gorm().Model(&model.RuleException{}).Where("id = ?", id).
		Update("enabled", next).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.reloadAfterWrite(c)
	response.OK(c, gin.H{"id": id, "enabled": next})
}

// ExceptionDelete 删除规则例外。
// 参数 c：Gin 上下文，路径参数 id 为例外 ID。
// 返回：无（200 返回 {"ok":true}；id 非法返回 400，数据库失败返回 500；删除后触发配置热重载，失败仅提示不影响已落库数据）。
// 路由：DELETE /api/exceptions/:id（需 rule:write 权限，经 CSRF/DemoGuard/审计中间件）。
func (s *Server) ExceptionDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	if err := s.Store.Gorm().Delete(&model.RuleException{}, id).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "rule_exception")
	c.Set(middleware.CtxAuditID, c.Param("id"))
	response.OK(c, gin.H{"ok": true})
}
