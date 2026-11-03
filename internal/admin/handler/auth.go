package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/auth"
	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// loginRequest 登录入参，驼峰对齐前端 Api.Auth.LoginParams。
type loginRequest struct {
	UserName string `json:"userName" binding:"required,min=2,max=64"`
	Password string `json:"password" binding:"required,min=4,max=128"`
}

// Login 登录。
func (s *Server) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "用户名与密码不能为空")
		return
	}

	ip := middleware.ClientIP(c)
	if ok, wait := s.LoginLimiter.Allowed(ip, req.UserName); !ok {
		response.Fail(c, http.StatusTooManyRequests, response.CodeTooMany,
			"该账号尝试过于频繁，请稍后再试 ("+strconv.Itoa(wait)+"s)")
		return
	}

	var u model.User
	err := s.Store.Gorm().Where("user_name = ? AND deleted_at = 0", req.UserName).First(&u).Error
	if err != nil || u.Status != 1 || !auth.VerifyPassword(u.PasswordHash, req.Password) {
		s.LoginLimiter.Fail(ip, req.UserName)
		s.Store.Audit(model.AuditLog{
			TS: store.NowMS(), Operator: req.UserName, ClientIP: ip,
			Action: "login", Object: "auth", Result: "deny", After: "密码错误",
		})
		response.BadRequest(c, "用户名或密码错误")
		return
	}

	s.LoginLimiter.Reset(ip, req.UserName)

	token, err := s.Auth.IssueAccess(&u)
	if err != nil {
		response.Internal(c, "签发令牌失败")
		return
	}
	refresh, jti, err := s.Auth.IssueRefresh(&u)
	if err != nil {
		response.Internal(c, "签发令牌失败")
		return
	}

	now := store.NowMS()
	if err := s.Store.Gorm().Create(&model.RefreshToken{
		UserID:    u.ID,
		JTI:       jti,
		ExpiresAt: now + int64(s.Auth.RefreshTTL()/time.Millisecond),
		ClientIP:  ip,
		UserAgent: truncateStr(c.Request.UserAgent(), 255),
		CreatedAt: now,
	}).Error; err != nil {
		s.failDB(c, err)
		return
	}

	s.Store.Gorm().Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"last_login_at": now,
		"last_login_ip": ip,
	})

	csrf := randomToken()
	s.setAuthCookies(c, token, csrf)

	s.Store.Audit(model.AuditLog{
		TS: now, Operator: u.UserName, ClientIP: ip,
		Action: "login", Object: "auth", Result: "ok",
	})

	c.Header("X-CSRF-Token", csrf)
	response.OK(c, gin.H{"token": token, "refreshToken": refresh})
}

// Logout 登出：吊销刷新令牌并清 Cookie。
func (s *Server) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = c.ShouldBindJSON(&req)

	if req.RefreshToken != "" {
		if claims, err := s.Auth.Parse(req.RefreshToken); err == nil {
			s.Store.Gorm().Model(&model.RefreshToken{}).
				Where("jti = ?", claims.ID).
				Update("revoked_at", store.NowMS())
		}
	}

	s.clearAuthCookies(c)
	response.OK(c, gin.H{"ok": true})
}

// Refresh 轮换刷新令牌。
func (s *Server) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		response.BadRequest(c, "缺少 refreshToken")
		return
	}

	claims, err := s.Auth.Parse(req.RefreshToken)
	if err != nil || claims.Purpose != auth.PurposeRefresh {
		response.Unauthorized(c, "刷新令牌无效")
		return
	}

	var rt model.RefreshToken
	if err := s.Store.Gorm().Where("jti = ?", claims.ID).First(&rt).Error; err != nil {
		response.Unauthorized(c, "刷新令牌已失效")
		return
	}
	now := store.NowMS()
	if rt.RevokedAt > 0 || rt.ExpiresAt < now {
		response.Unauthorized(c, "刷新令牌已失效")
		return
	}

	var u model.User
	if err := s.Store.Gorm().First(&u, rt.UserID).Error; err != nil || u.Status != 1 {
		response.Unauthorized(c, "账号不可用")
		return
	}

	// 旧令牌立即失效（一次一换，防重放）
	s.Store.Gorm().Model(&model.RefreshToken{}).Where("id = ?", rt.ID).Update("revoked_at", now)

	token, err := s.Auth.IssueAccess(&u)
	if err != nil {
		response.Internal(c, "签发令牌失败")
		return
	}
	refresh, jti, err := s.Auth.IssueRefresh(&u)
	if err != nil {
		response.Internal(c, "签发令牌失败")
		return
	}
	if err := s.Store.Gorm().Create(&model.RefreshToken{
		UserID:    u.ID,
		JTI:       jti,
		ExpiresAt: now + int64(s.Auth.RefreshTTL()/time.Millisecond),
		ClientIP:  middleware.ClientIP(c),
		CreatedAt: now,
	}).Error; err != nil {
		s.failDB(c, err)
		return
	}

	s.setAuthCookies(c, token, "")
	response.OK(c, gin.H{"token": token, "refreshToken": refresh})
}

// CSRFToken 下发双提交令牌（仅 Cookie 通道需要）。
func (s *Server) CSRFToken(c *gin.Context) {
	token := randomToken()
	s.setCSRFCookie(c, token)
	response.OK(c, gin.H{"token": token})
}

// Info 当前用户信息，前端登录后立刻调用。
func (s *Server) Info(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	var u model.User
	if err := s.Store.Gorm().First(&u, uid).Error; err != nil {
		response.Unauthorized(c, "账号不存在")
		return
	}

	var buttons []string
	_ = json.Unmarshal([]byte(u.Buttons), &buttons)
	if buttons == nil {
		buttons = []string{}
	}
	roles := []string{u.Role}
	if roles[0] == "" {
		roles = []string{"viewer"}
	}

	response.OK(c, gin.H{
		"userId":   u.ID,
		"userName": u.UserName,
		"nickName": u.NickName,
		"email":    u.Email,
		"avatar":   u.Avatar,
		"roles":    roles,
		"buttons":  buttons,
	})
}

// UpdatePassword 修改自己的口令。
func (s *Server) UpdatePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=8,max=128"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "新口令至少 8 位")
		return
	}
	uid := c.GetUint(middleware.CtxUserID)
	var u model.User
	if err := s.Store.Gorm().First(&u, uid).Error; err != nil {
		response.Unauthorized(c, "账号不存在")
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, req.OldPassword) {
		response.BadRequest(c, "原口令错误")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.Internal(c, "口令加密失败")
		return
	}
	if err := s.Store.Gorm().Model(&u).Updates(map[string]any{
		"password_hash": hash,
		"updated_at":    store.NowMS(),
	}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	// 改密后吊销全部刷新令牌，强制其它会话重新登录
	s.Store.Gorm().Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked_at = 0", uid).
		Update("revoked_at", store.NowMS())

	response.OK(c, gin.H{"ok": true})
}

// UserList 用户列表，供前端下拉选择器复用。
func (s *Server) UserList(c *gin.Context) {
	current, size := pageParams(c)
	kw := strings.TrimSpace(c.Query("userName"))

	q := s.Store.Gorm().Model(&model.User{}).Where("deleted_at = 0")
	if kw != "" {
		q = q.Where("user_name LIKE ?", "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		s.failDB(c, err)
		return
	}
	var users []model.User
	if err := q.Order("id ASC").Offset((current - 1) * size).Limit(size).Find(&users).Error; err != nil {
		s.failDB(c, err)
		return
	}

	records := make([]gin.H, 0, len(users))
	for _, u := range users {
		var buttons []string
		_ = json.Unmarshal([]byte(u.Buttons), &buttons)
		records = append(records, gin.H{
			"id":          u.ID,
			"userName":    u.UserName,
			"nickName":    u.NickName,
			"email":       u.Email,
			"avatar":      u.Avatar,
			"status":      strconv.Itoa(u.Status),
			"role":        u.Role,
			"buttons":     buttons,
			"lastLoginAt": u.LastLoginAt,
			"createdAt":   u.CreatedAt,
		})
	}
	response.OK(c, response.NewPage(records, current, size, total))
}

// ===== Cookie 与令牌工具 =====

func (s *Server) setAuthCookies(c *gin.Context, accessToken, csrf string) {
	maxAge := int(s.Auth.AccessTTL().Seconds())
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "waf_token",
		Value:    accessToken,
		Path:     "/",
		Domain:   s.Cfg.Admin.CookieDomain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.Cfg.Admin.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	if csrf != "" {
		s.setCSRFCookie(c, csrf)
	}
}

func (s *Server) setCSRFCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     s.Cfg.Admin.CSRF.CookieName,
		Value:    token,
		Path:     "/",
		Domain:   s.Cfg.Admin.CookieDomain,
		MaxAge:   int(s.Auth.AccessTTL().Seconds()),
		HttpOnly: false, // 前端 JS 需要读取
		Secure:   s.Cfg.Admin.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearAuthCookies(c *gin.Context) {
	for _, name := range []string{"waf_token", s.Cfg.Admin.CSRF.CookieName} {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Domain:   s.Cfg.Admin.CookieDomain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   s.Cfg.Admin.CookieSecure,
			SameSite: http.SameSiteStrictMode,
		})
	}
}

func randomToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
