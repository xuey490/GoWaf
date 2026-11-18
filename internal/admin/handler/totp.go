package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
	"gowaf/internal/totp"
)

// TotpStatus 返回当前用户的 TOTP 绑定状态（不含密钥本体）。
// 路由：GET /api/user/totp（需登录）。
// 参数 c：Gin 上下文。
// 返回：200 返回 {enabled: true/false}；用户不存在返回 401。
func (s *Server) TotpStatus(c *gin.Context) {
	var u model.User
	if err := s.Store.Gorm().First(&u, c.GetUint(middleware.CtxUserID)).Error; err != nil {
		response.Unauthorized(c, "账号不存在")
		return
	}
	response.OK(c, gin.H{"enabled": u.TOTPEnabled == 1 && u.TOTPSecret != ""})
}

// TotpSetup 生成新的 TOTP 密钥与绑定 URL。密钥在 enable 成功前只存为
// 待确认状态（写入 totp_secret 但 enabled 保持 0），防扫码前误锁。
// 路由：POST /api/user/totp/setup（需登录）。
// 参数 c：Gin 上下文。
// 返回：200 返回 {secret, otpauthUrl}；已开启 2FA 时要求先关闭再重新绑定。
func (s *Server) TotpSetup(c *gin.Context) {
	var u model.User
	if err := s.Store.Gorm().First(&u, c.GetUint(middleware.CtxUserID)).Error; err != nil {
		response.Unauthorized(c, "账号不存在")
		return
	}
	if u.TOTPEnabled == 1 {
		response.BadRequest(c, "两步验证已开启，请先关闭后重新绑定")
		return
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		response.Internal(c, "生成密钥失败")
		return
	}
	if err := s.Store.Gorm().Model(&u).Updates(map[string]any{
		"totp_secret": secret,
		"updated_at":  store.NowMS(),
	}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	issuer := "GoWAF"
	account := u.UserName
	response.OK(c, gin.H{
		"secret":     secret,
		"otpauthUrl": totp.ProvisionURL(issuer, account, secret),
	})
}

// TotpEnable 用验证码确认绑定：校验通过后 totp_enabled 置 1，下次登录生效。
// 路由：POST /api/user/totp/enable（需登录）。
// 参数 c：Gin 上下文，JSON 请求体 {code: "123456"}。
// 返回：200 返回 {ok:true}；未 setup、验证码错误返回 400。
func (s *Server) TotpEnable(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required,len=6"` // 动态码，6 位
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请输入 6 位动态码")
		return
	}
	var u model.User
	if err := s.Store.Gorm().First(&u, c.GetUint(middleware.CtxUserID)).Error; err != nil {
		response.Unauthorized(c, "账号不存在")
		return
	}
	if u.TOTPSecret == "" {
		response.BadRequest(c, "请先调用 setup 生成密钥")
		return
	}
	if !totp.Verify(u.TOTPSecret, req.Code) {
		response.BadRequest(c, "动态码不正确")
		return
	}
	if err := s.Store.Gorm().Model(&u).Updates(map[string]any{
		"totp_enabled": 1,
		"updated_at":   store.NowMS(),
	}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "user")
	response.OK(c, gin.H{"ok": true})
}

// TotpDisable 关闭两步验证：需当前有效动态码（防会话劫持后直接关闭）。
// 路由：POST /api/user/totp/disable（需登录）。
// 参数 c：Gin 上下文，JSON 请求体 {code: "123456"}。
// 返回：200 返回 {ok:true}；未开启或动态码错误返回 400。
func (s *Server) TotpDisable(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请输入 6 位动态码")
		return
	}
	var u model.User
	if err := s.Store.Gorm().First(&u, c.GetUint(middleware.CtxUserID)).Error; err != nil {
		response.Unauthorized(c, "账号不存在")
		return
	}
	if u.TOTPEnabled != 1 || u.TOTPSecret == "" {
		response.BadRequest(c, "两步验证未开启")
		return
	}
	if !totp.Verify(u.TOTPSecret, req.Code) {
		response.BadRequest(c, "动态码不正确")
		return
	}
	if err := s.Store.Gorm().Model(&u).Updates(map[string]any{
		"totp_enabled": 0,
		"totp_secret":  "",
		"updated_at":   store.NowMS(),
	}).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "user")
	response.OK(c, gin.H{"ok": true})
}

// totpRequired 标记登录响应中「需要两步验证码」的业务码。
const totpRequiredCode = 40102

// verifyLoginTotp 登录流程的 TOTP 校验环节。
// 已开启 2FA 且未携带正确动态码时返回需要验证码的业务响应。
// 参数 c：Gin 上下文；参数 u：已通过口令校验的用户；参数 code：登录请求中的动态码。
// 返回 bool：true=校验通过（或未开启），可继续签发令牌；false=已写入错误响应。
func (s *Server) verifyLoginTotp(c *gin.Context, u *model.User, code string) bool {
	if u.TOTPEnabled != 1 || u.TOTPSecret == "" {
		return true
	}
	if strings.TrimSpace(code) == "" {
		// 明确告诉前端「还差验证码这一步」：业务码 40102 + 提示字段
		response.Fail(c, 400, totpRequiredCode, "该账号已开启两步验证，请输入动态码")
		return false
	}
	if !totp.Verify(u.TOTPSecret, code) {
		response.BadRequest(c, "动态码不正确")
		return false
	}
	return true
}
