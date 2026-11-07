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
// 参数 name：用户指定的 slug 或原始包名。
// 返回：仅含小写字母/数字/连字符的安全文件名片段（最长 60 字符）；
// 清洗后为空时返回 "pkg-<毫秒时间戳>"。
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-") // 非法字符统一替换为连字符
	s = strings.Trim(s, "-")            // 去掉首尾连字符
	if s == "" {
		return fmt.Sprintf("pkg-%d", store.NowMS())
	}
	if len(s) > 60 {
		s = s[:60] // 截断到 60 字符，与 slug 列长度对齐
	}
	return s
}

// SecLangPackages 返回已导入的 SecLang 规则包列表。
// 路由：GET /api/seclang/packages（写操作组：CSRF/演示拦截/审计，未挂按钮级权限校验）。
// 参数 c：Gin 上下文。
// 返回：无（200 返回 {"list":包元信息列表,"total":总数}；数据库错误返回 500）。
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
// 路由：POST /api/seclang/import（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，JSON 正文为下方入参结构，text 为 .conf 规则包全文。
// 返回：无（200 返回 {"slug","name","rules"}；text 为空或编译失败 400，
// 落盘/数据库失败 500）。
func (s *Server) SecLangImport(c *gin.Context) {
	var in struct {
		Name    string `json:"name"`    // 包名；留空时取首行注释兜底
		Slug    string `json:"slug"`    // 包标识（文件名片段）；留空时由包名生成
		Vendor  string `json:"vendor"`  // 维护方
		Version string `json:"version"` // 版本号；新建时缺省为 "1"
		Note    string `json:"note"`    // 备注说明
		Text    string `json:"text"`    // 规则包全文（必填）
		Enabled *int   `json:"enabled"` // 启用状态指针：nil 视为启用
		DryRun  bool   `json:"dryRun"`  // true 只编译校验，不落盘不落库
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

	// 独立编译整包校验：任何一条编译不过都整包拒绝，避免脏包入库后拖垮全局重载
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
	// slug 已存在则视为升级覆盖，只更新元信息；否则新建记录
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
// 路由：POST /api/seclang/:id/toggle（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，路径参数 id 为包 ID，JSON 正文为 {"enabled":1|0}。
// 返回：无（200 返回 {"affected":1}；包 ID 无效或包不存在 400，数据库错误 500）。
func (s *Server) SecLangToggle(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的包 ID")
		return
	}
	var in struct {
		Enabled int `json:"enabled"` // 目标状态：1 启用 / 0 停用
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	// 更新时顺带清空 last_error，给包一次重新生效的机会
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
	s.RT.InvalidateCRS() // 包启用状态变化，强制下次重载重建 CRS 引擎
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "seclang_toggle")
	response.OK(c, gin.H{"affected": res.RowsAffected})
}

// SecLangDelete 删除一个 SecLang 规则包（文件 + 元信息）。
// 路由：DELETE /api/seclang/:id（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，路径参数 id 为包 ID。
// 返回：无（200 返回 {"deleted":被删包的 slug}；包 ID 无效或包不存在 400，数据库错误 500）。
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
	// 删除落盘的 .conf 文件；文件已不存在不视为错误，忽略返回值
	_ = os.Remove(filepath.Join(s.Cfg.BaseDir(), "seclang", row.Slug+".conf"))
	s.RT.InvalidateCRS() // 包被删除，强制下次重载重建 CRS 引擎
	s.reloadAfterWrite(c)
	c.Set(middleware.CtxAuditObj, "seclang_delete:"+row.Slug)
	response.OK(c, gin.H{"deleted": row.Slug})
}
