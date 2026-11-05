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

// loginRequest 登录入参，字段驼峰命名对齐前端 Api.Auth.LoginParams。
type loginRequest struct {
	// 用户名：必填，长度 2~64 字符。
	UserName string `json:"userName" binding:"required,min=2,max=64"`
	// 口令明文：必填，长度 4~128 字符。
	Password string `json:"password" binding:"required,min=4,max=128"`
}

// Login 处理登录请求：校验用户名口令，签发访问/刷新令牌并写入会话 Cookie。
// 路由：POST /api/auth/login（公开接口，前置登录失败限流中间件）。
// 参数 c：Gin 上下文，JSON 请求体为 loginRequest。
// 返回：无（200 返回 token/refreshToken；参数非法或凭据错误返回 400，触发限流返回 429）。
func (s *Server) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "用户名与密码不能为空")
		return
	}

	// 以客户端真实 IP 为限流维度之一，超限时拒绝并提示剩余等待秒数
	ip := middleware.ClientIP(c)
	if ok, wait := s.LoginLimiter.Allowed(ip, req.UserName); !ok {
		response.Fail(c, http.StatusTooManyRequests, response.CodeTooMany,
			"该账号尝试过于频繁，请稍后再试 ("+strconv.Itoa(wait)+"s)")
		return
	}

	// 用户不存在、已删除、被禁用或口令不匹配，统一按「凭据错误」处理，
	// 不区分具体原因，避免向攻击者暴露账号是否存在
	var u model.User
	err := s.Store.Gorm().Where("user_name = ? AND deleted_at = 0", req.UserName).First(&u).Error
	if err != nil || u.Status != 1 || !auth.VerifyPassword(u.PasswordHash, req.Password) {
		// 计入一次登录失败，并写 deny 审计日志
		s.LoginLimiter.Fail(ip, req.UserName)
		s.Store.Audit(model.AuditLog{
			TS: store.NowMS(), Operator: req.UserName, ClientIP: ip,
			Action: "login", Object: "auth", Result: "deny", After: "密码错误",
		})
		response.BadRequest(c, "用户名或密码错误")
		return
	}

	// 登录成功，清零该 IP+用户名的失败计数
	s.LoginLimiter.Reset(ip, req.UserName)

	// 分别签发短期访问令牌与长期刷新令牌
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
	// 刷新令牌记录落库：JTI 唯一索引，供 Refresh 轮换与登出吊销时校验
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

	// 回写最近登录时间与 IP；更新失败不影响登录结果
	s.Store.Gorm().Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"last_login_at": now,
		"last_login_ip": ip,
	})

	// 生成 CSRF 令牌随 Cookie 下发，并通过响应头返回一份供前端读取
	csrf := randomToken()
	s.setAuthCookies(c, token, csrf)

	s.Store.Audit(model.AuditLog{
		TS: now, Operator: u.UserName, ClientIP: ip,
		Action: "login", Object: "auth", Result: "ok",
	})

	c.Header("X-CSRF-Token", csrf)
	response.OK(c, gin.H{"token": token, "refreshToken": refresh})
}

// Logout 处理登出请求：吊销刷新令牌并清除会话 Cookie。
// 路由：POST /api/auth/logout（公开接口，无需登录态）。
// 参数 c：Gin 上下文，JSON 请求体可选携带 refreshToken。
// 返回：无（200 返回 {"ok":true}）。
func (s *Server) Logout(c *gin.Context) {
	// 请求体可省略，解析失败也照常执行清 Cookie 流程
	var req struct {
		RefreshToken string `json:"refreshToken"` // 待吊销的刷新令牌，可为空
	}
	_ = c.ShouldBindJSON(&req)

	// 只有令牌能通过签名解析出 JTI 才能定位到落库记录；伪造或过期的令牌无需吊销
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

// Refresh 轮换刷新令牌：校验旧刷新令牌有效后吊销它，并签发新的访问/刷新令牌。
// 路由：POST /api/auth/refresh（公开接口）。
// 参数 c：Gin 上下文，JSON 请求体携带 refreshToken。
// 返回：无（200 返回新的 token/refreshToken；令牌无效/已吊销/已过期或账号不可用返回 401）。
func (s *Server) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken"` // 待轮换的刷新令牌，必填
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		response.BadRequest(c, "缺少 refreshToken")
		return
	}

	// 签名校验失败，或用途不是 refresh（如拿访问令牌来刷新）一律拒绝
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
	// 已吊销或已过期的令牌一律拒绝
	if rt.RevokedAt > 0 || rt.ExpiresAt < now {
		response.Unauthorized(c, "刷新令牌已失效")
		return
	}

	// 关联用户必须仍存在且处于启用状态
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
	// 新刷新令牌落库，供下一轮轮换与吊销校验
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

// CSRFToken 下发新的 CSRF 双提交令牌：写入 Cookie 并在响应体返回一份。
// 路由：GET /api/auth/csrf（需登录）。
// 参数 c：Gin 上下文。
// 返回：无（200 返回 {"token":"..."}）。
func (s *Server) CSRFToken(c *gin.Context) {
	token := randomToken()
	s.setCSRFCookie(c, token)
	response.OK(c, gin.H{"token": token})
}

// Info 返回当前登录用户的资料与权限（角色、按钮码），前端登录后立刻调用。
// 路由：GET /api/user/info（需登录）。
// 参数 c：Gin 上下文，用户 ID 取自鉴权中间件写入的请求上下文。
// 返回：无（200 返回用户资料 JSON；用户不存在返回 401）。
func (s *Server) Info(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	var u model.User
	if err := s.Store.Gorm().First(&u, uid).Error; err != nil {
		response.Unauthorized(c, "账号不存在")
		return
	}

	// buttons 列存 JSON 字符串，解析失败按无按钮权限处理
	var buttons []string
	_ = json.Unmarshal([]byte(u.Buttons), &buttons)
	if buttons == nil {
		buttons = []string{}
	}
	// 角色为空时回落为 viewer，保证前端路由守卫始终有角色可判断
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

// UpdatePassword 修改当前登录用户自己的口令，成功后吊销其全部刷新令牌。
// 路由：PUT /api/user/password（需登录）。
// 参数 c：Gin 上下文，JSON 请求体携带 oldPassword/newPassword。
// 返回：无（200 返回 {"ok":true}；参数非法或原口令错误返回 400）。
func (s *Server) UpdatePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`               // 原口令：必填
		NewPassword string `json:"newPassword" binding:"required,min=8,max=128"` // 新口令：必填，8~128 位
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
	// 先校验原口令，防止会话被劫持后直接改密
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

// UserList 分页查询用户列表，供前端下拉选择器等场景复用。
// 路由：GET /api/user/list（需登录）。
// 参数 c：Gin 上下文，查询参数 current/size 控制分页，userName 模糊过滤用户名。
// 返回：无（200 返回分页结构 records/total；数据库失败返回 500）。
func (s *Server) UserList(c *gin.Context) {
	current, size := pageParams(c)
	kw := strings.TrimSpace(c.Query("userName"))

	// 只查询未删除用户，用户名支持模糊匹配
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

// setAuthCookies 把访问令牌写入 HttpOnly Cookie，csrf 非空时额外下发 CSRF Cookie。
// 参数 c：Gin 上下文；accessToken：访问令牌；csrf：CSRF 令牌，空串表示不下发。
// 返回：无。
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

// setCSRFCookie 写入 CSRF 双提交 Cookie；前端 JS 需要读取该值回传请求头，故不设 HttpOnly。
// 参数 c：Gin 上下文；token：CSRF 令牌值。
// 返回：无。
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

// clearAuthCookies 通过 MaxAge=-1 让访问令牌与 CSRF Cookie 立即失效。
// 参数 c：Gin 上下文。
// 返回：无。
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

// randomToken 生成 16 字节随机数的 hex 字符串，用作 CSRF 令牌。
// 参数：无。
// 返回：32 字符 hex 串；系统随机源失败时退化为纳秒时间戳 hex，保证返回值非空。
func randomToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

// truncateStr 按字节截断字符串到指定长度，用于把 User-Agent 等入参裁进数据库列宽。
// 参数 s：原始字符串；n：最大字节数。
// 返回：不超过 n 字节的字符串；原串更短时原样返回。
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
