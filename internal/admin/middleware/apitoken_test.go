package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// newTokenStore 打开临时库并写入一个已知的 API Token，供认证中间件测试使用。
// 参数 t：测试实例；参数 plain：明文令牌；参数 scopes：权限域。
// 返回 *store.Store：已就绪的存储实例。
func newTokenStore(t *testing.T, plain, scopes string) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sum := sha256.Sum256([]byte(plain))
	tk := model.APIToken{
		Name: "test", TokenHash: hex.EncodeToString(sum[:]), Scopes: scopes,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := st.Gorm().Create(&tk).Error; err != nil {
		t.Fatalf("写入令牌失败: %v", err)
	}
	return st
}

// runAPITokenAuth 以给定 Authorization 头执行一次 APITokenAuth 中间件。
// 参数 t：测试实例；参数 st：存储；参数 authHeader：Authorization 头的值（可为空）。
// 返回 *gin.Context：执行后的上下文（用其 GetString 读认证状态）。
func runAPITokenAuth(t *testing.T, st *store.Store, authHeader string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("GET", "/api/v1/overview", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	c.Request = req
	APITokenAuth(st)(c) // 未命中令牌时 c.Next() 走空链路，安全
	return c
}

// TestAPITokenAuthValidToken 有效令牌：写上下文并跳过 JWT。
func TestAPITokenAuthValidToken(t *testing.T) {
	st := newTokenStore(t, "gwt_plain-test-token", "read,write")
	c := runAPITokenAuth(t, st, "Bearer gwt_plain-test-token")
	if got := c.GetString(CtxAuthFrom); got != "api_token" {
		t.Fatalf("CtxAuthFrom = %s, 期望 api_token", got)
	}
	if got := c.GetString(CtxAPIScopes); got != "read,write" {
		t.Fatalf("CtxAPIScopes = %s", got)
	}
	if got := c.GetString(CtxUserName); got != "api:test" {
		t.Fatalf("CtxUserName = %s", got)
	}
}

// TestAPITokenAuthUnknownHeader 非令牌凭据：透传给 JWT 链路（不写认证上下文）。
func TestAPITokenAuthUnknownHeader(t *testing.T) {
	st := newTokenStore(t, "gwt_plain-test-token", "read")
	c := runAPITokenAuth(t, st, "Bearer not-a-token")
	if got := c.GetString(CtxAuthFrom); got != "" {
		t.Fatalf("未命中令牌不应写认证上下文, 实际 %s", got)
	}
}

// TestAPITokenAuthNoHeader 无 Authorization 头：透传。
func TestAPITokenAuthNoHeader(t *testing.T) {
	st := newTokenStore(t, "gwt_plain-test-token", "read")
	c := runAPITokenAuth(t, st, "")
	if got := c.GetString(CtxAuthFrom); got != "" {
		t.Fatalf("无凭据不应写认证上下文, 实际 %s", got)
	}
}

// TestRequireAPIScope 验证作用域门禁：read-only 令牌被挡、write 令牌放行、JWT 放行。
func TestRequireAPIScope(t *testing.T) {
	cases := []struct {
		from   string // CtxAuthFrom
		scopes string
		pass   bool
	}{
		{"api_token", "read", false},
		{"api_token", "read,write", true},
		{"header", "read", true}, // JWT 会话不受作用域约束
		{"cookie", "read", true},
	}
	for _, tc := range cases {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/api/v1/bans", nil)
		c.Set(CtxAuthFrom, tc.from)
		c.Set(CtxAPIScopes, tc.scopes)
		RequireAPIScope(ScopeWrite)(c)
		if c.IsAborted() == tc.pass {
			t.Fatalf("from=%s scopes=%s 放行=%v, 期望 %v", tc.from, tc.scopes, !c.IsAborted(), tc.pass)
		}
	}
}
