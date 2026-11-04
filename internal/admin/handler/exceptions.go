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

// ExceptionList 例外列表。
func (s *Server) ExceptionList(c *gin.Context) {
	q := s.Store.Gorm().Model(&model.RuleException{})
	if site := strings.TrimSpace(c.Query("site")); site != "" {
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

type exceptionInput struct {
	Site       string `json:"site"`
	RuleID     string `json:"ruleId"`
	Category   string `json:"category"`
	PathPrefix string `json:"pathPrefix"`
	Param      string `json:"param"`
	Note       string `json:"note"`
	Enabled    *int   `json:"enabled"`
}

func validateException(in *exceptionInput) string {
	if in.Site == "" && in.RuleID == "" && in.PathPrefix == "" && in.Param == "" {
		return "站点/规则/路径/参数至少要填一项，否则等于全局关闭检测"
	}
	return ""
}

// ExceptionCreate 新增例外。通常从攻击事件的「误报」按钮一键生成。
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
	enabled := 1
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

// ExceptionUpdate 修改例外。
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

// ExceptionToggle 启用/停用。
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

// ExceptionDelete 删除例外。
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
