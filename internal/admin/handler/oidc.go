package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/auth"
	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// OIDC 控制台统一认证（对标雷池 CE 的 OIDC 登录，8.6.0 起全版本支持）。
//
// 流程：前端 GET /api/auth/oidc/start 拿到 IdP 授权页地址 → 浏览器跳转完成
// 登录 → IdP 302 回 /api/auth/oidc/callback?code&state → 这里换 access token、
// 拉 userinfo 取 sub/email → 按 "oidc:<sub>" 匹配或自动建号 → 签发本系统
// JWT + Cookie → 302 回控制台首页。
//
// 安全设计：
//   - state 一次性、10 分钟 TTL（防 CSRF 与重放）；
//   - OIDC 用户名强制 "oidc:<sub>" 命名空间——绝不用 email 匹配既有本地
//     账号（IdP 可控 email 值，用它关联本地账号等于把控制台登录权交给 IdP）；
//   - 自动建号走最低权限角色（oidc.role，默认 viewer）；
//   - OIDC 用户口令为随机串（无法被口令登录，只能经 IdP 或管理员重置）。
//
// 依赖：IdP 必须支持 authorization_code + userinfo endpoint（OIDC 标配）。
// 出于零依赖（CGO_ENABLED=0 纯 Go）考虑，token 交换与 userinfo 为手工
// HTTP 实现，不引 oauth2/oidc 客户端库。

// oidcProvider OIDC 发现文档中用到的字段。
type oidcProvider struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// oidcUserInfo userinfo 响应中用到的字段。
type oidcUserInfo struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
}

// oidcState 单次授权流程的状态记录。
type oidcState struct {
	expiresAt int64
}

// oidcDiscoverCache 发现文档缓存：issuer → (provider, 拉取时间)。
var (
	oidcStateMu     sync.Mutex
	oidcStates      = map[string]oidcState{}
	oidcDiscoverMu  sync.Mutex
	oidcDiscoverMap = map[string]struct {
		p    oidcProvider
		at   time.Time
		fail string
	}{}
)

const (
	oidcStateTTL    = 10 * time.Minute
	oidcDiscoverTTL = time.Hour
)

// OIDCStatus 报告统一认证是否启用（登录页据此决定是否显示 OIDC 按钮）。
// 路由：GET /api/auth/oidc/status（公开接口）。
// 参数 c：Gin 上下文。
// 返回：200 返回 {"enabled":bool}。
func (s *Server) OIDCStatus(c *gin.Context) {
	response.OK(c, gin.H{
		"enabled": s.RT.SettingBool(runtime.KeyOIDCEnabled, false),
	})
}

// OIDCStart 生成 IdP 授权页跳转地址。
// 路由：GET /api/auth/oidc/start（公开接口）。
// 参数 c：Gin 上下文。
// 返回：200 返回 {"redirect": "..."}；未启用或 IdP 配置不可用返回 400/502。
func (s *Server) OIDCStart(c *gin.Context) {
	if !s.RT.SettingBool(runtime.KeyOIDCEnabled, false) {
		response.BadRequest(c, "统一认证未启用")
		return
	}
	issuer := strings.TrimSpace(s.RT.Setting(runtime.KeyOIDCIssuer, ""))
	clientID := strings.TrimSpace(s.RT.Setting(runtime.KeyOIDCClientID, ""))
	clientSecret := s.RT.Setting(runtime.KeyOIDCClientSecret, "")
	if issuer == "" || clientID == "" || clientSecret == "" {
		response.BadRequest(c, "OIDC 配置不完整（issuer/client_id/client_secret）")
		return
	}
	p, fail := s.oidcDiscover(issuer)
	if fail != "" {
		response.Fail(c, http.StatusBadGateway, response.CodeInternal, "IdP 发现失败: "+fail)
		return
	}
	redirectURI := s.oidcRedirectURI(c)

	// state：随机 32 字节 hex，一次性消费
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		response.Internal(c, "生成 state 失败")
		return
	}
	state := hex.EncodeToString(buf)
	oidcStateMu.Lock()
	oidcStates[state] = oidcState{expiresAt: time.Now().Add(oidcStateTTL).UnixMilli()}
	oidcStateMu.Unlock()

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	scopes := strings.TrimSpace(s.RT.Setting(runtime.KeyOIDCScopes, "openid profile email"))
	q.Set("scope", scopes)
	q.Set("state", state)

	response.OK(c, gin.H{"redirect": p.AuthorizationEndpoint + "?" + q.Encode()})
}

// OIDCCallback 处理 IdP 回调：换 token → userinfo → 匹配/建号 → 签发会话 → 回控制台。
// 路由：GET /api/auth/oidc/callback?code&state（公开接口，IdP 302 过来）。
// 参数 c：Gin 上下文。
// 返回：成功 302 到控制台首页（Cookie 已下发）；失败 302 到登录页并带 error 参数。
func (s *Server) OIDCCallback(c *gin.Context) {
	loginFail := func(msg string) {
		c.Redirect(http.StatusFound, "/login?oidcError="+url.QueryEscape(msg))
	}
	if !s.RT.SettingBool(runtime.KeyOIDCEnabled, false) {
		loginFail("统一认证未启用")
		return
	}
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		loginFail("回调缺少 code/state")
		return
	}
	// 校验并消费 state（一次性）
	oidcStateMu.Lock()
	st, ok := oidcStates[state]
	if ok {
		delete(oidcStates, state)
	}
	oidcStateMu.Unlock()
	if !ok || time.Now().UnixMilli() > st.expiresAt {
		loginFail("state 已失效，请重新发起登录")
		return
	}

	issuer := strings.TrimSpace(s.RT.Setting(runtime.KeyOIDCIssuer, ""))
	clientID := strings.TrimSpace(s.RT.Setting(runtime.KeyOIDCClientID, ""))
	clientSecret := s.RT.Setting(runtime.KeyOIDCClientSecret, "")
	p, fail := s.oidcDiscover(issuer)
	if fail != "" {
		loginFail("IdP 发现失败")
		return
	}

	tok, fail := oidcTokenExchange(p.TokenEndpoint, clientID, clientSecret, s.oidcRedirectURI(c), code)
	if fail != "" {
		s.Store.Audit(model.AuditLog{TS: store.NowMS(), Operator: "oidc", ClientIP: middleware.ClientIP(c),
			Action: "login", Object: "auth", Result: "deny", After: "token 交换失败: " + fail})
		loginFail("令牌交换失败")
		return
	}
	ui, fail := oidcFetchUserinfo(p.UserinfoEndpoint, tok)
	if fail != "" {
		loginFail("获取用户信息失败")
		return
	}
	if ui.Sub == "" {
		loginFail("userinfo 缺少 sub")
		return
	}
	userName := "oidc:" + ui.Sub

	// 匹配既有 OIDC 用户；缺失时按 auto_create 决定建号或拒绝
	var u model.User
	err := s.Store.Gorm().Where("user_name = ? AND deleted_at = 0", userName).First(&u).Error
	if err != nil {
		if !s.RT.SettingBool(runtime.KeyOIDCAutoCreate, true) {
			s.Store.Audit(model.AuditLog{TS: store.NowMS(), Operator: userName, ClientIP: middleware.ClientIP(c),
				Action: "login", Object: "auth", Result: "deny", After: "用户不存在且未开启自动建号"})
			loginFail("账号不存在，请联系管理员")
			return
		}
		role := s.RT.Setting(runtime.KeyOIDCRole, RoleViewer)
		if !validRoles[role] || role == RoleAdmin {
			// 建号永远不自动给 admin——配置错误也不能放权
			role = RoleViewer
		}
		buttons, _ := json.Marshal(defaultButtons(role))
		hash, _ := authHashRandom()
		u = model.User{
			UserName:     userName,
			PasswordHash: hash,
			NickName:     firstNonEmpty(ui.PreferredUsername, ui.Name, ui.Email, ui.Sub),
			Email:        strings.TrimSpace(ui.Email),
			Role:         role,
			Buttons:      string(buttons),
			Status:       1,
			CreatedAt:    store.NowMS(),
		}
		if err := s.Store.Gorm().Create(&u).Error; err != nil {
			loginFail("创建账号失败")
			return
		}
		s.Store.Audit(model.AuditLog{TS: store.NowMS(), Operator: currentOperator(c), ClientIP: middleware.ClientIP(c),
			Action: "user.create", Object: userName, Result: "ok", After: "oidc 自动建号 role=" + role})
	}
	if u.Status != 1 {
		loginFail("账号已停用")
		return
	}

	// 与 Login 相同的会话签发：access + Cookie + CSRF。refresh 仅落库供
	// 吊销审计（OIDC 会话不下发 refresh token，access 过期后重新走 IdP
	// 登录——SSO 惯例，且避免把长期令牌经 URL/页面暴露给前端）。
	token, err1 := s.Auth.IssueAccess(&u)
	refresh, jti, err2 := s.Auth.IssueRefresh(&u)
	_ = refresh
	if err1 != nil || err2 != nil {
		loginFail("签发令牌失败")
		return
	}
	now := store.NowMS()
	s.Store.Gorm().Create(&model.RefreshToken{
		UserID: u.ID, JTI: jti,
		ExpiresAt: now + int64(s.Auth.RefreshTTL()/time.Millisecond),
		ClientIP:  middleware.ClientIP(c), UserAgent: truncateStr(c.Request.UserAgent(), 255),
		CreatedAt: now,
	})
	s.Store.Gorm().Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"last_login_at": now, "last_login_ip": middleware.ClientIP(c),
	})
	csrf := randomToken()
	s.setAuthCookies(c, token, csrf)
	s.Store.Audit(model.AuditLog{TS: now, Operator: u.UserName, ClientIP: middleware.ClientIP(c),
		Action: "login", Object: "auth", Result: "ok", After: "oidc"})
	c.Redirect(http.StatusFound, "/")
}

// oidcRedirectURI 计算回调地址：显式配置优先，否则按请求的 scheme/host 推导。
func (s *Server) oidcRedirectURI(c *gin.Context) string {
	if v := strings.TrimSpace(s.RT.Setting(runtime.KeyOIDCRedirectURL, "")); v != "" {
		return v
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + c.Request.Host + "/api/auth/oidc/callback"
}

// oidcDiscover 拉取并缓存 OIDC 发现文档（1 小时）。
func (s *Server) oidcDiscover(issuer string) (oidcProvider, string) {
	oidcDiscoverMu.Lock()
	cached, ok := oidcDiscoverMap[issuer]
	oidcDiscoverMu.Unlock()
	if ok && time.Since(cached.at) < oidcDiscoverTTL && cached.fail == "" {
		return cached.p, ""
	}
	wellKnown := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(wellKnown)
	if err != nil {
		s.oidcCache(issuer, oidcProvider{}, time.Now(), err.Error())
		return oidcProvider{}, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		s.oidcCache(issuer, oidcProvider{}, time.Now(), fmt.Sprintf("HTTP %d", resp.StatusCode))
		return oidcProvider{}, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	var p oidcProvider
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p); err != nil {
		s.oidcCache(issuer, oidcProvider{}, time.Now(), "发现文档解析失败")
		return oidcProvider{}, "发现文档解析失败"
	}
	if p.AuthorizationEndpoint == "" || p.TokenEndpoint == "" {
		s.oidcCache(issuer, oidcProvider{}, time.Now(), "发现文档缺少端点")
		return oidcProvider{}, "发现文档缺少端点"
	}
	s.oidcCache(issuer, p, time.Now(), "")
	return p, ""
}

// oidcCache 写发现缓存（fail 非空时也缓存失败结果，TTL 内不重试）。
func (s *Server) oidcCache(issuer string, p oidcProvider, at time.Time, fail string) {
	oidcDiscoverMu.Lock()
	oidcDiscoverMap[issuer] = struct {
		p    oidcProvider
		at   time.Time
		fail string
	}{p, at, fail}
	oidcDiscoverMu.Unlock()
}

// oidcTokenExchange 用授权码换 access token（application/x-www-form-urlencoded）。
// 返回 access_token 与失败原因。
func oidcTokenExchange(tokenURL, clientID, clientSecret, redirectURI, code string) (string, string) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(tokenURL, form)
	if err != nil {
		return "", err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Sprintf("token 端点 HTTP %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", "token 响应缺少 access_token"
	}
	return tok.AccessToken, ""
}

// oidcFetchUserinfo 用 access token 拉取用户信息。
func oidcFetchUserinfo(userinfoURL, accessToken string) (oidcUserInfo, string) {
	if userinfoURL == "" {
		return oidcUserInfo{}, "发现文档缺少 userinfo_endpoint"
	}
	req, _ := http.NewRequest("GET", userinfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return oidcUserInfo{}, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return oidcUserInfo{}, fmt.Sprintf("userinfo HTTP %d", resp.StatusCode)
	}
	var ui oidcUserInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&ui); err != nil {
		return oidcUserInfo{}, "userinfo 解析失败"
	}
	return ui, ""
}

// authHashRandom 生成 OIDC 用户的不可口令登录哈希（随机 32 字节口令的 bcrypt，
// 明文不留存——OIDC 用户只能经 IdP 登录或由管理员重置口令）。
func authHashRandom() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return auth.HashPassword(hex.EncodeToString(buf))
}

// firstNonEmpty 返回第一个非空串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
