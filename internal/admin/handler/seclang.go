package handler

import (
	"fmt"
	"os"
	"path/filepath"
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

/* ============================================================ SecLang 规则包 */

// slugRe 把包名清洗成安全的文件名片段（仅小写字母/数字/连字符）。
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify 生成 slug；输入全是非法字符时退化为时间戳。
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return fmt.Sprintf("pkg-%d", store.NowMS())
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// SecLangPackages 返回已导入的 SecLang 规则包列表。
func (s *Server) SecLangPackages(c *gin.Context) {
	var rows []model.SeclangPackage
	if err := s.Store.Gorm().Order("id ASC").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// SecLangImport 导入一个 SecLang 规则包（.conf 文本）。
//
// 流程：独立编译校验（失败整包拒绝）→ 落盘 data/seclang/<slug>.conf →
// upsert 元信息 → 强制重建 CRS 引擎。slug 相同视为升级覆盖。
// dryRun=true 只校验，返回编译结果与规则条数。
func (s *Server) SecLangImport(c *gin.Context) {
	var in struct {
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Vendor  string `json:"vendor"`
		Version string `json:"version"`
		Note    string `json:"note"`
		Text    string `json:"text"`
		Enabled *int   `json:"enabled"`
		DryRun  bool   `json:"dryRun"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		response.BadRequest(c, "text 不能为空")
		return
	}
	// 防呆：误把 YAML 规则包塞进来时给出明确提示，而不是一堆编译错误。
	head := strings.TrimSpace(in.Text)
	if !strings.Contains(head, "SecRule") && !strings.Contains(head, "SecAction") &&
		!strings.Contains(head, "SecMarker") && !strings.Contains(head, "Include ") {
		response.BadRequest(c, "这不是 SecLang 规则包（缺少 SecRule/SecAction 指令），YAML 规则请用「导入规则包」")
		return
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		// 允许不带包名导入：取第一行注释或 slug 化的固定前缀。
		if strings.HasPrefix(head, "#") {
			line := strings.SplitN(strings.TrimLeft(head, "# "), "\n", 2)[0]
			name = strings.TrimSpace(line)
		}
		if name == "" {
			name = "SecLang 包"
		}
		if len(name) > 60 {
			name = name[:60]
		}
	}
	slug := slugify(in.Slug)
	if slug == "" {
		slug = slugify(name)
	}

	count, err := detect.ValidateSecLang(in.Text)
	if err != nil {
		response.BadRequest(c, "SecLang 编译失败: "+err.Error())
		return
	}

	if in.DryRun {
		response.OK(c, gin.H{"dryRun": true, "slug": slug, "name": name, "rules": count})
		return
	}

	dir := filepath.Join(s.Cfg.BaseDir(), "seclang")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		response.Internal(c, "创建规则包目录失败: "+err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(dir, slug+".conf"), []byte(in.Text), 0o644); err != nil {
		response.Internal(c, "写入规则包文件失败: "+err.Error())
		return
	}

	enabled := 1
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	now := store.NowMS()
	operator := c.GetString(middleware.CtxUserName)

	var old model.SeclangPackage
	if s.Store.Gorm().Where("slug = ?", slug).First(&old).Error == nil {
		updates := map[string]any{
			"name": name, "vendor": in.Vendor, "version": in.Version,
			"rule_count": count, "enabled": enabled, "last_error": "",
			"note": in.Note, "updated_at": now,
		}
		if err := s.Store.Gorm().Model(&model.SeclangPackage{}).Where("id = ?", old.ID).Updates(updates).Error; err != nil {
			s.failDB(c, err)
			return
		}
	} else {
		row := model.SeclangPackage{
			Name: name, Slug: slug, Vendor: in.Vendor, Version: in.Version,
			RuleCount: count, Enabled: enabled, Note: in.Note,
			CreatedBy: operator, CreatedAt: now, UpdatedAt: now,
		}
		if row.Version == "" {
			row.Version = "1"
		}
		if err := s.Store.Gorm().Create(&row).Error; err != nil {
			s.failDB(c, err)
			return
		}
	}

	// 规则包内容变了必须强制重建 CRS 引擎，否则 reloadCRS 会因
	// mode/paranoia 未变而跳过。
	s.RT.InvalidateCRS()
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "seclang_import:"+slug)
	response.OK(c, gin.H{"slug": slug, "name": name, "rules": count})
}

// SecLangToggle 启用/停用一个 SecLang 规则包。
func (s *Server) SecLangToggle(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的包 ID")
		return
	}
	var in struct {
		Enabled int `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	res := s.Store.Gorm().Model(&model.SeclangPackage{}).Where("id = ?", id).
		Updates(map[string]any{"enabled": in.Enabled, "last_error": "", "updated_at": store.NowMS()})
	if res.Error != nil {
		s.failDB(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		response.BadRequest(c, "规则包不存在")
		return
	}
	s.RT.InvalidateCRS()
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "seclang_toggle")
	response.OK(c, gin.H{"affected": res.RowsAffected})
}

// SecLangDelete 删除一个 SecLang 规则包（文件 + 元信息）。
func (s *Server) SecLangDelete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的包 ID")
		return
	}
	var row model.SeclangPackage
	if s.Store.Gorm().First(&row, id).Error != nil {
		response.BadRequest(c, "规则包不存在")
		return
	}
	if err := s.Store.Gorm().Delete(&model.SeclangPackage{}, row.ID).Error; err != nil {
		s.failDB(c, err)
		return
	}
	_ = os.Remove(filepath.Join(s.Cfg.BaseDir(), "seclang", row.Slug+".conf"))
	s.RT.InvalidateCRS()
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "seclang_delete:"+row.Slug)
	response.OK(c, gin.H{"deleted": row.Slug})
}
