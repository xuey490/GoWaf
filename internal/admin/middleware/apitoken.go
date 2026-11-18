package middleware

// API Token（Bearer）认证：供 OpenAPI 与 MCP 客户端免交互访问控制面。
//
// 认证模型（与 JWT 会话共存）：
//   1. 请求带 Authorization 头时先尝试按 API Token 匹配（SHA-256 摘要查库）；
//   2. 匹配成功：身份写入上下文（CtxAuthFrom="api_token"）并中止后续链路
//      （跳过 JWT 校验）；不匹配或无头：交还给 Auth 中间件走 JWT/Cookie 流程；
//   3. 写接口由 RequireAPIScope 再挡一道：read-only 令牌一律 403，JWT 会话不受影响。
//
// 安全设计：
//   - 库里只存 SHA-256 摘要，明文令牌仅在创建时返回一次；
//   - 摘要查找走唯一索引，O(1)；恒定时间比较不必要（哈希查找无时序侧信道，
//     攻击者无法通过响应时间枚举 256 位摘要空间）；
//   - last_used_at 异步回写，不阻塞请求路径，也不与 SQLite 单写连接争锁。

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// CtxAPIScopes API Token 的权限域快照（"read" 或 "read,write"），RequireAPIScope 读取。
const CtxAPIScopes = "api_scopes"

// ScopeWrite 写权限域的标识值。
const ScopeWrite = "write"

// APITokenAuth 创建 API Token 认证中间件，必须注册在 Auth 之前。
// 参数 st：存储层，按摘要查令牌。
// 返回：Gin 中间件；令牌命中时写上下文并跳过 JWT，否则透传给后续认证链。
func APITokenAuth(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" {
			c.Next()
			return
		}
		raw := strings.TrimSpace(h)
		if len(raw) > 7 && strings.EqualFold(raw[:7], "bearer ") {
			raw = strings.TrimSpace(raw[7:])
		}
		if raw == "" {
			c.Next()
			return
		}

		sum := sha256.Sum256([]byte(raw))
		hash := hex.EncodeToString(sum[:])

		var t model.APIToken
		if err := st.Gorm().Where("token_hash = ? AND revoked_at = 0", hash).First(&t).Error; err != nil {
			// 不是有效的 API Token：交还 JWT 链路（JWT 会话用户的令牌不会与
			// 任何摘要冲突）。查询失败同样交还，由 Auth 给出标准 401。
			c.Next()
			return
		}

		c.Set(CtxUserID, uint(0))
		c.Set(CtxUserName, "api:"+t.Name)
		c.Set(CtxRole, "api")
		c.Set(CtxAuthFrom, "api_token")
		c.Set(CtxAPIScopes, t.Scopes)

		// 最近使用时间异步回写：单写连接的 SQLite 不能在请求路径上排队写。
		tokenID := t.ID
		db := st.Gorm()
		go func() {
			defer func() { _ = recover() }()
			if err := db.Model(&model.APIToken{}).Where("id = ?", tokenID).
				Update("last_used_at", time.Now().UnixMilli()).Error; err != nil {
				log.Printf("[apitoken] 回写最近使用时间失败: %v", err)
			}
		}()

		c.Next()
		c.Abort() // 已认证：跳过后续的 JWT 校验
	}
}

// RequireAPIScope 对经 API Token 认证的请求校验权限域；JWT 会话用户直接放行。
// 参数 scope：所需的权限域（当前仅 write）。
// 返回：Gin 中间件；read-only 令牌访问写接口返回 403。
func RequireAPIScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(CtxAuthFrom) != "api_token" {
			c.Next()
			return
		}
		if strings.Contains(c.GetString(CtxAPIScopes), scope) {
			c.Next()
			return
		}
		response.Forbidden(c, "API Token 缺少 "+scope+" 权限（请在控制台重建 read,write 令牌）")
	}
}
