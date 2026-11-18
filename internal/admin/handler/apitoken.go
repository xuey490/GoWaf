package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

/* ================================================== API 访问令牌 */

// apiTokenPrefix 明文令牌前缀：便于在使用处一眼识别凭据类型（与 JWT 区分）。
const apiTokenPrefix = "gwt_"

// TokenList 返回全部 API 令牌（不含摘要与明文）。
// 参数 c：Gin 上下文，无查询参数。
// 返回：list 按 id 倒序的令牌列表（名称/权限域/创建/最近使用/吊销状态）。
// 路由：GET /api/tokens。
func (s *Server) TokenList(c *gin.Context) {
	var rows []model.APIToken
	if err := s.Store.Gorm().Order("id DESC").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows})
}

// TokenCreate 生成一个 API 令牌。明文只在本次响应返回一次，库里只存摘要。
// 参数 c：Gin 上下文，请求体 {name, scopes}；scopes 取 "read"（默认）或 "read,write"。
// 返回：200 与 {token: 明文令牌, ...}；名称为空或权限域非法返回 400。
// 路由：POST /api/tokens（要求 settings:write 权限，经 CSRF/DemoGuard/审计）。
func (s *Server) TokenCreate(c *gin.Context) {
	var in struct {
		Name   string `json:"name" binding:"required"`
		Scopes string `json:"scopes"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "name 必填")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 64 {
		response.BadRequest(c, "令牌名称必须为 1~64 字符")
		return
	}
	scopes := in.Scopes
	if scopes == "" {
		scopes = "read"
	}
	if scopes != "read" && scopes != "read,write" {
		response.BadRequest(c, "scopes 只能是 read 或 read,write")
		return
	}

	// 24 字节随机（192 位熵）→ 48 字符 hex，前缀 gwt_ 标识凭据类型。
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		s.failDB(c, err)
		return
	}
	plain := apiTokenPrefix + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))

	t := model.APIToken{
		Name:      name,
		TokenHash: hex.EncodeToString(sum[:]),
		Scopes:    scopes,
		CreatedBy: c.GetString(middleware.CtxUserName),
		CreatedAt: store.NowMS(),
	}
	if err := s.Store.Gorm().Create(&t).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "api_token")
	c.Set(middleware.CtxAuditID, name)
	response.OK(c, gin.H{
		"id": t.ID, "name": t.Name, "scopes": t.Scopes,
		"token": plain, // 仅此一次返回明文，关掉页面后无法再查看
	})
}

// TokenRevoke 吊销一个 API 令牌（保留记录供审计，revoked_at 非 0 即失效）。
// 参数 c：Gin 上下文，路由参数 id。
// 返回：成功返回 200；令牌不存在或已吊销返回 404。
// 路由：DELETE /api/tokens/:id（要求 settings:write 权限）。
func (s *Server) TokenRevoke(c *gin.Context) {
	q := s.Store.Gorm().Model(&model.APIToken{}).
		Where("id = ? AND revoked_at = 0", c.Param("id"))
	res := q.Update("revoked_at", time.Now().UnixMilli())
	if res.Error != nil {
		s.failDB(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		response.NotFound(c, "令牌不存在或已吊销")
		return
	}
	c.Set(middleware.CtxAuditObj, "api_token")
	c.Set(middleware.CtxAuditID, c.Param("id"))
	response.OK(c, gin.H{"ok": true})
}
