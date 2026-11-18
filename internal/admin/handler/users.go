package handler

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/auth"
	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// 角色集合与默认按钮权限。
// admin 拥有全部按钮；viewer 只读（无任何写按钮）；demo 沿用旧语义（只读）。
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
	RoleDemo   = "demo"
)

// validRoles 可分配的角色白名单。
var validRoles = map[string]bool{RoleAdmin: true, RoleViewer: true, RoleDemo: true}

// defaultButtons 角色的默认按钮权限集合。
// 参数 role：角色标识。返回 []string：按钮权限列表。
func defaultButtons(role string) []string {
	switch role {
	case RoleAdmin:
		return append([]string(nil), store.AllButtons...)
	default: // viewer / demo：只读，无写按钮
		return []string{}
	}
}

// ===== 请求体 =====

type userCreateRequest struct {
	UserName string   `json:"userName"`
	Password string   `json:"password"`
	NickName string   `json:"nickName"`
	Email    string   `json:"email"`
	Role     string   `json:"role"`
	Buttons  []string `json:"buttons"`
}

type userUpdateRequest struct {
	NickName *string   `json:"nickName"`
	Email    *string   `json:"email"`
	Role     *string   `json:"role"`
	Buttons  *[]string `json:"buttons"`
	Status   *int      `json:"status"`
}

type userResetPasswordRequest struct {
	Password string `json:"password"`
}

// UserCreate 创建用户（role:write 权限）。
// 路由：POST /api/users。
// 参数 c：Gin 上下文，JSON 请求体 {userName,password,nickName,email,role,buttons}。
// 返回：201 返回新用户摘要；用户名重复/角色非法/口令过短返回 400。
func (s *Server) UserCreate(c *gin.Context) {
	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "用户名与密码不能为空")
		return
	}
	name := strings.TrimSpace(req.UserName)
	if len(name) < 2 || len(name) > 64 || strings.ContainsAny(name, " \t\r\n:@/") {
		response.BadRequest(c, "用户名需 2~64 字符，且不含空白与 :@/ 字符")
		return
	}
	if len(req.Password) < 8 {
		response.BadRequest(c, "口令至少 8 位")
		return
	}
	role := req.Role
	if role == "" {
		role = RoleViewer
	}
	if !validRoles[role] {
		response.BadRequest(c, "角色只能是 admin/viewer/demo")
		return
	}
	buttons := req.Buttons
	if buttons == nil {
		buttons = defaultButtons(role)
	}
	buttonsJSON, err := json.Marshal(buttons)
	if err != nil {
		response.Internal(c, "序列化权限失败")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Internal(c, "口令加密失败")
		return
	}
	u := model.User{
		UserName:     name,
		PasswordHash: hash,
		NickName:     strings.TrimSpace(req.NickName),
		Email:        strings.TrimSpace(req.Email),
		Role:         role,
		Buttons:      string(buttonsJSON),
		Status:       1,
		CreatedAt:    store.NowMS(),
	}
	if err := s.Store.Gorm().Create(&u).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			response.BadRequest(c, "用户名已存在")
			return
		}
		s.failDB(c, err)
		return
	}
	s.Store.Audit(model.AuditLog{
		TS: store.NowMS(), Operator: currentOperator(c), ClientIP: middleware.ClientIP(c),
		Action: "user.create", Object: name, Result: "ok", After: "role=" + role,
	})
	response.OK(c, gin.H{"id": u.ID, "userName": u.UserName, "role": u.Role})
}

// UserUpdate 更新用户资料/角色/按钮/状态（role:write 权限）。
// 路由：PUT /api/users/:id。
// 参数 c：Gin 上下文，JSON 请求体字段均可选。
// 返回：200 返回更新后的用户摘要；改自己角色/停自己/清空最后一个 admin 返回 400。
func (s *Server) UserUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "用户 ID 非法")
		return
	}
	var req userUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求体非法")
		return
	}
	var u model.User
	if err := s.Store.Gorm().Where("id = ? AND deleted_at = 0", id).First(&u).Error; err != nil {
		response.NotFound(c, "用户不存在")
		return
	}

	updates := map[string]any{"updated_at": store.NowMS()}
	if req.NickName != nil {
		updates["nick_name"] = strings.TrimSpace(*req.NickName)
	}
	if req.Email != nil {
		updates["email"] = strings.TrimSpace(*req.Email)
	}
	if req.Role != nil {
		role := *req.Role
		if !validRoles[role] {
			response.BadRequest(c, "角色只能是 admin/viewer/demo")
			return
		}
		if role != RoleAdmin && u.Role == RoleAdmin && u.ID == currentAdminSelfID(c) {
			response.BadRequest(c, "不能修改自己的角色")
			return
		}
		if role != RoleAdmin && u.Role == RoleAdmin && s.isLastAdmin(u.ID) {
			response.BadRequest(c, "系统至少需要保留一个 admin 账号")
			return
		}
		updates["role"] = role
		// 角色变更且未显式给按钮时，按钮集合跟随角色默认值
		if req.Buttons == nil {
			b, _ := json.Marshal(defaultButtons(role))
			updates["buttons"] = string(b)
		}
	}
	if req.Buttons != nil {
		b, _ := json.Marshal(*req.Buttons)
		updates["buttons"] = string(b)
	}
	if req.Status != nil {
		st := *req.Status
		if st != 0 && st != 1 {
			response.BadRequest(c, "status 只能是 0（停用）或 1（启用）")
			return
		}
		if st != 1 && u.ID == currentAdminSelfID(c) {
			response.BadRequest(c, "不能停用自己的账号")
			return
		}
		if st != 1 && u.Role == RoleAdmin && s.isLastAdmin(u.ID) {
			response.BadRequest(c, "系统至少需要保留一个 admin 账号")
			return
		}
		updates["status"] = st
	}
	if err := s.Store.Gorm().Model(&model.User{}).Where("id = ?", u.ID).Updates(updates).Error; err != nil {
		s.failDB(c, err)
		return
	}
	s.Store.Audit(model.AuditLog{
		TS: store.NowMS(), Operator: currentOperator(c), ClientIP: middleware.ClientIP(c),
		Action: "user.update", Object: u.UserName, Result: "ok",
	})
	response.OK(c, gin.H{"id": u.ID, "userName": u.UserName})
}

// UserDelete 删除用户（软删除，role:write 权限）。
// 路由：DELETE /api/users/:id。
// 参数 c：Gin 上下文。
// 返回：200 返回 {"ok":true}；删除自己/最后一个 admin 返回 400。
func (s *Server) UserDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "用户 ID 非法")
		return
	}
	var u model.User
	if err := s.Store.Gorm().Where("id = ? AND deleted_at = 0", id).First(&u).Error; err != nil {
		response.NotFound(c, "用户不存在")
		return
	}
	if u.ID == currentAdminSelfID(c) {
		response.BadRequest(c, "不能删除自己的账号")
		return
	}
	if u.Role == RoleAdmin && s.isLastAdmin(u.ID) {
		response.BadRequest(c, "系统至少需要保留一个 admin 账号")
		return
	}
	if err := s.Store.Gorm().Model(&model.User{}).Where("id = ?", u.ID).
		Updates(map[string]any{"deleted_at": store.NowMS(), "status": 0, "updated_at": store.NowMS()}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	// 吊销该用户全部刷新令牌，保证已签发会话立即失效
	s.Store.Gorm().Where("user_id = ?", u.ID).Delete(&model.RefreshToken{})
	s.Store.Audit(model.AuditLog{
		TS: store.NowMS(), Operator: currentOperator(c), ClientIP: middleware.ClientIP(c),
		Action: "user.delete", Object: u.UserName, Result: "ok",
	})
	response.OK(c, gin.H{"ok": true})
}

// UserResetPassword 管理员重置用户口令（role:write 权限）。
// 路由：POST /api/users/:id/reset-password。
// 参数 c：Gin 上下文，JSON 请求体 {password}。
// 返回：200 返回 {"ok":true}；password 为空时自动生成随机口令并返回明文（仅此一次）。
func (s *Server) UserResetPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "用户 ID 非法")
		return
	}
	var req userResetPasswordRequest
	_ = c.ShouldBindJSON(&req)
	pwd := req.Password
	auto := false
	if pwd == "" {
		pwd = store.RandomPassword()
		auto = true
	}
	if len(pwd) < 8 {
		response.BadRequest(c, "口令至少 8 位")
		return
	}
	var u model.User
	if err := s.Store.Gorm().Where("id = ? AND deleted_at = 0", id).First(&u).Error; err != nil {
		response.NotFound(c, "用户不存在")
		return
	}
	hash, err := auth.HashPassword(pwd)
	if err != nil {
		response.Internal(c, "口令加密失败")
		return
	}
	if err := s.Store.Gorm().Model(&model.User{}).Where("id = ?", u.ID).
		Updates(map[string]any{"password_hash": hash, "updated_at": store.NowMS()}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	// 口令变更后吊销全部刷新令牌
	s.Store.Gorm().Where("user_id = ?", u.ID).Delete(&model.RefreshToken{})
	s.Store.Audit(model.AuditLog{
		TS: store.NowMS(), Operator: currentOperator(c), ClientIP: middleware.ClientIP(c),
		Action: "user.reset_password", Object: u.UserName, Result: "ok",
	})
	if auto {
		response.OK(c, gin.H{"ok": true, "password": pwd})
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// currentAdminSelfID 从会话里取当前操作者用户 ID（不可解析时返回 0）。
func currentAdminSelfID(c *gin.Context) uint {
	return c.GetUint(middleware.CtxUserID)
}

// currentOperator 当前操作者用户名（审计用）。
func currentOperator(c *gin.Context) string {
	return c.GetString(middleware.CtxUserName)
}

// isLastAdmin 判断指定 admin 用户之外是否还有其他启用中的 admin 账号。
func (s *Server) isLastAdmin(excludeID uint) bool {
	var n int64
	s.Store.Gorm().Model(&model.User{}).
		Where("deleted_at = 0 AND status = 1 AND role = ? AND id <> ?", RoleAdmin, excludeID).
		Count(&n)
	return n == 0
}
