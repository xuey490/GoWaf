package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// newUsersTestServer 建临时库 + 已登录 admin 的测试上下文工厂。
func newUsersTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("开库失败: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.SeedAdmin("root"); err != nil {
		t.Fatalf("种子管理员失败: %v", err)
	}
	s := New(Deps{Store: st})
	return s, "root"
}

// ctxAsAdmin 构造带 admin 会话上下文的请求记录器。
func ctxAsAdmin(c *gin.Context) {
	c.Set(middleware.CtxUserID, uint(1))
	c.Set(middleware.CtxUserName, "root")
	c.Set(middleware.CtxRole, "admin")
}

// doJSON 以 admin 身份调用 handler。
func doJSON(s *Server, method, path string, body any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	ctxAsAdmin(c)
	c.Params = gin.Params{}
	if strings.HasSuffix(path, "/reset-password") {
		// /api/users/<id>/reset-password：取倒数第二段
		parts := strings.Split(path, "/")
		c.Params = append(c.Params, gin.Param{Key: "id", Value: parts[len(parts)-2]})
	} else if i := strings.LastIndex(path, "/"); i >= 0 && strings.HasPrefix(path, "/api/users/") {
		c.Params = append(c.Params, gin.Param{Key: "id", Value: path[i+1:]})
	}
	switch {
	case method == http.MethodPost && strings.HasSuffix(path, "/reset-password"):
		s.UserResetPassword(c)
	case method == http.MethodPost && path == "/api/users":
		s.UserCreate(c)
	case method == http.MethodPut:
		s.UserUpdate(c)
	case method == http.MethodDelete:
		s.UserDelete(c)
	}
	return w
}

// findUserByName 按用户名查用户。
func findUserByName(t *testing.T, s *Server, name string) model.User {
	t.Helper()
	var u model.User
	if err := s.Store.Gorm().Where("user_name = ? AND deleted_at = 0", name).First(&u).Error; err != nil {
		t.Fatalf("查用户 %s 失败: %v", name, err)
	}
	return u
}

func TestUserCreateDefaults(t *testing.T) {
	s, _ := newUsersTestServer(t)
	w := doJSON(s, "POST", "/api/users", map[string]any{"userName": "alice", "password": "secret123"})
	if w.Code != 200 {
		t.Fatalf("创建失败: %d %s", w.Code, w.Body.String())
	}
	u := findUserByName(t, s, "alice")
	if u.Role != RoleViewer {
		t.Fatalf("默认角色应为 viewer: %s", u.Role)
	}
	if strings.Contains(u.Buttons, "rule:write") {
		t.Fatalf("viewer 不应带写权限按钮: %s", u.Buttons)
	}
	// 重复用户名
	w2 := doJSON(s, "POST", "/api/users", map[string]any{"userName": "alice", "password": "secret123"})
	if w2.Code != 400 {
		t.Fatalf("重复用户名应 400: %d", w2.Code)
	}
	// 弱口令
	w3 := doJSON(s, "POST", "/api/users", map[string]any{"userName": "bob", "password": "123"})
	if w3.Code != 400 {
		t.Fatalf("弱口令应 400: %d", w3.Code)
	}
}

func TestUserCreateAdminGetsAllButtons(t *testing.T) {
	s, _ := newUsersTestServer(t)
	doJSON(s, "POST", "/api/users", map[string]any{"userName": "boss", "password": "secret123", "role": "admin"})
	u := findUserByName(t, s, "boss")
	if !strings.Contains(u.Buttons, "user:write") {
		t.Fatalf("admin 应含 user:write 按钮: %s", u.Buttons)
	}
}

func TestUserGuards(t *testing.T) {
	s, _ := newUsersTestServer(t)
	doJSON(s, "POST", "/api/users", map[string]any{"userName": "ops", "password": "secret123", "role": "admin"})
	admin2 := findUserByName(t, s, "ops")

	// root（ID=1）是唯一 admin 时：不能把自己降级/停用/删除
	w := doJSON(s, "PUT", "/api/users/1", map[string]any{"role": "viewer"})
	if w.Code != 400 {
		t.Fatalf("最后一个 admin 不能改角色: %d", w.Code)
	}
	w = doJSON(s, "PUT", "/api/users/1", map[string]any{"status": 0})
	if w.Code != 400 {
		t.Fatalf("不能停用自己: %d", w.Code)
	}
	w = doJSON(s, "DELETE", "/api/users/1", nil)
	if w.Code != 400 {
		t.Fatalf("不能删除自己: %d", w.Code)
	}

	// 有两个 admin 时：root 可以被改角色
	doJSON(s, "PUT", "/api/users/"+itoa(int(admin2.ID)), map[string]any{"role": "viewer"})
	u := findUserByName(t, s, "ops")
	if u.Role != RoleViewer {
		t.Fatalf("第二个 admin 应可降级: %s", u.Role)
	}
	// viewer 改回 admin 且按钮跟随默认
	doJSON(s, "PUT", "/api/users/"+itoa(int(admin2.ID)), map[string]any{"role": "admin"})
	u = findUserByName(t, s, "ops")
	if !strings.Contains(u.Buttons, "user:write") {
		t.Fatalf("改回 admin 后按钮应重置: %s", u.Buttons)
	}
}

func TestUserResetPasswordAuto(t *testing.T) {
	s, _ := newUsersTestServer(t)
	doJSON(s, "POST", "/api/users", map[string]any{"userName": "carol", "password": "secret123"})
	carol := findUserByName(t, s, "carol")
	w := doJSON(s, "POST", "/api/users/"+itoa(int(carol.ID))+"/reset-password", map[string]any{})
	if w.Code != 200 {
		t.Fatalf("重置失败: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if pwd, _ := resp.Data["password"].(string); len(pwd) < 8 {
		t.Fatalf("自动口令应 ≥8 位: %q", resp.Data["password"])
	}
}

// itoa 简单整数转字符串（避免测试里引 strconv 全套）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
