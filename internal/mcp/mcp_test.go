package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== 假控制台 =====

// capturedReq 记录假控制台收到的一次请求,供测试断言。
type capturedReq struct {
	Method string // HTTP 方法
	Path   string // 请求路径(不含查询串)
	Query  string // 原始查询串
	Auth   string // Authorization 头
	Body   string // 请求体原文
}

// fakeConsole 假控制台:httptest server,请求先记账再交给测试注册的 mux 处理。
type fakeConsole struct {
	srv  *httptest.Server
	mux  *http.ServeMux
	mu   sync.Mutex
	reqs []capturedReq
}

// newFakeConsole 启动假控制台,返回实例与可注册 handler 的 mux;
// 未注册路径由 mux 返回文本 404,可用于验证错误传播。
func newFakeConsole(t *testing.T) (*fakeConsole, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	fc := &fakeConsole{mux: mux}
	fc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		// 记账后把读掉的 body 还回去,后续 handler 才能重新解析请求体
		r.Body = io.NopCloser(bytes.NewReader(body))
		fc.mu.Lock()
		fc.reqs = append(fc.reqs, capturedReq{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Auth: r.Header.Get("Authorization"), Body: string(body),
		})
		fc.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(fc.srv.Close)
	return fc, mux
}

// captured 取第 i 次(从 0 开始)请求记录;请求数不足时 fatal。
func (fc *fakeConsole) captured(t *testing.T, i int) capturedReq {
	t.Helper()
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if i >= len(fc.reqs) {
		t.Fatalf("假控制台只收到 %d 个请求,取第 %d 个失败", len(fc.reqs), i)
	}
	return fc.reqs[i]
}

// size 返回已收到的请求数。
func (fc *fakeConsole) size() int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return len(fc.reqs)
}

// okJSON 输出标准成功包装 {code:200, msg:"ok", data:<data>}。
func okJSON(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"code":200,"msg":"ok","data":%s}`, data)
}

// failJSON 输出业务失败包装,HTTP 状态与业务码通常一致。
func failJSON(w http.ResponseWriter, status, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"code":%d,"msg":%q}`, code, msg)
}

// ===== 会话辅助 =====

// testResp 测试侧解析的响应帧。
type testResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// testSession 持有与 mcp server 相连的管道,逐行交换 JSON-RPC 帧。
type testSession struct {
	in io.WriteCloser // server 的 stdin 写端
	sc *bufio.Scanner // server 的 stdout 读端
}

// readTimeout 是单次读写协议帧的等待上限,防止 server 挂死拖垮测试。
const readTimeout = 5 * time.Second

// startSession 设置环境变量并启动 Run,返回测试会话;测试结束后自动关停。
func startSession(t *testing.T, url, token string) *testSession {
	t.Helper()
	t.Setenv(EnvURL, url)
	t.Setenv(EnvToken, token)

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	t.Cleanup(func() { _ = stdinW.Close() })

	go func() {
		// Run 的返回值不影响断言:测试通过协议帧与假控制台请求记录验证行为
		_ = Run(stdinR, stdoutW, "test-1.0.0")
		_ = stdoutW.Close() // Run 结束后让读端拿到 EOF
	}()

	sc := bufio.NewScanner(stdoutR)
	sc.Buffer(make([]byte, 0, 64*1024), scanBufSize)
	return &testSession{in: stdinW, sc: sc}
}

// writeLine 写一行文本到 server 的 stdin,超时 fatal。
func (s *testSession) writeLine(t *testing.T, line string) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(s.in, line+"\n")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("写入请求失败: %v", err)
		}
	case <-time.After(readTimeout):
		t.Fatal("写入请求超时(server 未读取 stdin)")
	}
}

// readResponse 读取一帧响应,超时或流提前结束均 fatal。
func (s *testSession) readResponse(t *testing.T) *testResp {
	t.Helper()
	type scanRes struct {
		resp *testResp
		err  error
	}
	ch := make(chan scanRes, 1)
	go func() {
		if !s.sc.Scan() {
			ch <- scanRes{err: s.sc.Err()}
			return
		}
		var r testResp
		if e := json.Unmarshal(s.sc.Bytes(), &r); e != nil {
			ch <- scanRes{err: fmt.Errorf("响应不是合法 JSON: %w", e)}
			return
		}
		ch <- scanRes{resp: &r}
	}()
	select {
	case res := <-ch:
		if res.err != nil || res.resp == nil {
			t.Fatalf("读取响应失败或流已结束: %v", res.err)
		}
		return res.resp
	case <-time.After(readTimeout):
		t.Fatal("等待响应超时")
		return nil
	}
}

// call 写一行带 id 的请求并读取一帧响应;params 为空串时不带 params 字段。
func (s *testSession) call(t *testing.T, id, method, params string) *testResp {
	t.Helper()
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"method":%q`, id, method)
	if params != "" {
		line += fmt.Sprintf(`,"params":%s`, params)
	}
	s.writeLine(t, line+"}")
	return s.readResponse(t)
}

// assertNoError 断言响应无 JSON-RPC error。
func assertNoError(t *testing.T, resp *testResp) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("不应返回协议错误,实际 %d: %s", resp.Error.Code, resp.Error.Message)
	}
}

// toolResult 解析 tools/call 响应:返回首段 text 与 isError 标记。
// 响应带 JSON-RPC error 时 fatal(协议错误与工具错误是两类失败)。
func toolResult(t *testing.T, resp *testResp) (string, bool) {
	t.Helper()
	assertNoError(t, resp)
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("解析 result 失败: %v(原文 %s)", err, resp.Result)
	}
	if len(res.Content) == 0 || res.Content[0].Type != "text" {
		t.Fatalf("content 应包含 text 内容块,实际: %s", resp.Result)
	}
	return res.Content[0].Text, res.IsError
}

// ===== 协议测试 =====

// TestInitialize 验证握手返回:协议版本、能力声明与服务信息。
func TestInitialize(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	resp := s.call(t, "init-1", "initialize",
		`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}`)
	assertNoError(t, resp)

	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools json.RawMessage `json:"tools"`
		} `json:"capabilities"`
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("解析 initialize 结果失败: %v(原文 %s)", err, resp.Result)
	}
	if res.ProtocolVersion != protocolVersion {
		t.Errorf("protocolVersion = %q, 期望 %q", res.ProtocolVersion, protocolVersion)
	}
	if len(res.Capabilities.Tools) == 0 {
		t.Errorf("capabilities 应声明 tools,实际: %s", resp.Result)
	}
	if res.ServerInfo.Name != serverName {
		t.Errorf("serverInfo.name = %q, 期望 %q", res.ServerInfo.Name, serverName)
	}
	if res.ServerInfo.Version != "test-1.0.0" {
		t.Errorf("serverInfo.version = %q, 期望 %q", res.ServerInfo.Version, "test-1.0.0")
	}
}

// TestPing 验证 ping 返回空结果对象。
func TestPing(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	resp := s.call(t, "p-1", "ping", "")
	assertNoError(t, resp)
	if strings.TrimSpace(string(resp.Result)) != "{}" {
		t.Errorf("ping 结果应为空对象,实际: %s", resp.Result)
	}
	if fc.size() != 0 {
		t.Errorf("ping 不应访问控制台,实际收到 %d 个请求", fc.size())
	}
}

// TestNotificationIgnored 验证通知(含未知通知)不产生响应:通知之后
// 发出的 ping 应是读到的第一帧响应。
func TestNotificationIgnored(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	s.writeLine(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	s.writeLine(t, `{"jsonrpc":"2.0","method":"mcp/unknown-notification"}`)
	s.writeLine(t, `{"jsonrpc":"2.0","id":"ping-1","method":"ping"}`)

	resp := s.readResponse(t)
	if string(resp.ID) != `"ping-1"` {
		t.Fatalf("通知不应产生响应,首帧响应 ID = %s", resp.ID)
	}
	assertNoError(t, resp)
}

// TestParseError 验证非法 JSON 行返回 -32700 且 ID 为 null。
func TestParseError(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	s.writeLine(t, "{oops")
	resp := s.readResponse(t)
	if resp.Error == nil || resp.Error.Code != errCodeParse {
		t.Fatalf("应返回 -32700,实际: %+v", resp.Error)
	}
	if strings.TrimSpace(string(resp.ID)) != "null" {
		t.Errorf("解析错误的响应 ID 应为 null,实际: %s", resp.ID)
	}
}

// TestUnknownMethod 验证未知请求方法返回 -32601。
func TestUnknownMethod(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	resp := s.call(t, "u-1", "foo/bar", "")
	if resp.Error == nil || resp.Error.Code != errCodeMethodNotFound {
		t.Fatalf("应返回 -32601,实际: %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "foo/bar") {
		t.Errorf("错误信息应包含方法名,实际: %s", resp.Error.Message)
	}
}

// TestInvalidJSONRPCVersion 验证 jsonrpc 版本错误返回 -32600。
func TestInvalidJSONRPCVersion(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	s.writeLine(t, `{"jsonrpc":"1.0","id":"v-1","method":"ping"}`)
	resp := s.readResponse(t)
	if resp.Error == nil || resp.Error.Code != errCodeInvalidReq {
		t.Fatalf("应返回 -32600,实际: %+v", resp.Error)
	}
}

// ===== 工具清单测试 =====

// wantToolNames 是全部工具名,与 toolRegistry 一一对应。
var wantToolNames = []string{
	"waf_overview", "waf_timeseries", "waf_events", "waf_event_detail",
	"waf_sites", "waf_bans", "waf_ban_ip", "waf_unban_ip",
	"waf_rules", "waf_rule_test", "waf_settings",
}

// TestToolsList 验证工具清单完整且每个工具都有中文说明与对象型 Schema。
func TestToolsList(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	resp := s.call(t, "tl-1", "tools/list", "")
	assertNoError(t, resp)

	var res struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type string `json:"type"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("解析 tools/list 结果失败: %v", err)
	}
	if len(res.Tools) != len(wantToolNames) {
		t.Fatalf("工具数 = %d, 期望 %d", len(res.Tools), len(wantToolNames))
	}
	seen := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		seen[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("工具 %s 缺少说明", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("工具 %s 的 inputSchema.type = %q, 期望 object", tool.Name, tool.InputSchema.Type)
		}
	}
	for _, name := range wantToolNames {
		if !seen[name] {
			t.Errorf("工具清单缺少 %s", name)
		}
	}
}

// TestUnknownTool 验证调用未注册工具返回 -32602。
func TestUnknownTool(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	resp := s.call(t, "ut-1", "tools/call", `{"name":"no_such_tool"}`)
	if resp.Error == nil || resp.Error.Code != errCodeInvalidParams {
		t.Fatalf("应返回 -32602,实际: %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "no_such_tool") {
		t.Errorf("错误信息应包含工具名,实际: %s", resp.Error.Message)
	}
}

// TestToolCallMissingName 验证缺 name 的 tools/call 返回 -32602。
func TestToolCallMissingName(t *testing.T) {
	fc, _ := newFakeConsole(t)
	s := startSession(t, fc.srv.URL, "tok-1")

	resp := s.call(t, "mn-1", "tools/call", `{}`)
	if resp.Error == nil || resp.Error.Code != errCodeInvalidParams {
		t.Fatalf("应返回 -32602,实际: %+v", resp.Error)
	}
}

// ===== 工具调用测试(经假控制台) =====

// TestCallOverview 验证 overview 工具:携带 Bearer 令牌、查询串拼装与 data 解包。
func TestCallOverview(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/dashboard/overview", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			failJSON(w, http.StatusUnauthorized, 401, "未登录")
			return
		}
		okJSON(w, `{"total":1234,"blocked":56,"qps":1.5,"blockRate":0.045,"mode":"block"}`)
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	text, isErr := toolResult(t, s.call(t, "ov-1", "tools/call",
		`{"name":"waf_overview","arguments":{"site":"shop","range":"24h"}}`))
	if isErr {
		t.Fatalf("overview 不应报错: %s", text)
	}
	if !strings.Contains(text, `"total":1234`) || !strings.Contains(text, `"blockRate":0.045`) {
		t.Errorf("text 应为 data 解包结果,实际: %s", text)
	}

	req := fc.captured(t, 0)
	if req.Method != "GET" || req.Path != pathOverview {
		t.Errorf("请求 = %s %s, 期望 GET %s", req.Method, req.Path, pathOverview)
	}
	if !strings.Contains(req.Query, "site=shop") || !strings.Contains(req.Query, "range=24h") {
		t.Errorf("查询串应包含过滤参数,实际: %s", req.Query)
	}
	if req.Auth != "Bearer tok-1" {
		t.Errorf("Authorization = %q, 期望 %q", req.Auth, "Bearer tok-1")
	}
}

// TestCallBanIP 验证封禁工具:请求体对齐 BanCreate、ttl 映射 duration、成功结果解包。
func TestCallBanIP(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("POST /api/bans", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			IP       string `json:"ip"`
			Duration int64  `json:"duration"`
			Reason   string `json:"reason"`
			Scope    string `json:"scope"`
			Site     string `json:"site"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			failJSON(w, http.StatusBadRequest, 400, "参数格式错误")
			return
		}
		// 回显关键字段,便于断言 data 解包
		okJSON(w, fmt.Sprintf(
			`{"id":7,"expiresAt":1234567890,"scope":%q,"site":%q,"operator":"admin"}`, in.Scope, in.Site))
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	text, isErr := toolResult(t, s.call(t, "ban-1", "tools/call",
		`{"name":"waf_ban_ip","arguments":{"ip":"1.2.3.4","ttl":3600,"reason":"恶意扫描","scope":"site","site":"shop"}}`))
	if isErr {
		t.Fatalf("ban_ip 不应报错: %s", text)
	}
	if !strings.Contains(text, `"id":7`) {
		t.Errorf("text 应包含封禁结果,实际: %s", text)
	}

	req := fc.captured(t, 0)
	if req.Method != "POST" || req.Path != pathBans {
		t.Errorf("请求 = %s %s, 期望 POST %s", req.Method, req.Path, pathBans)
	}
	var body banCreateBody
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		t.Fatalf("请求体不是合法 JSON: %v(%s)", err, req.Body)
	}
	if body.IP != "1.2.3.4" {
		t.Errorf("ip = %q, 期望 %q", body.IP, "1.2.3.4")
	}
	if body.Duration != 3600 {
		t.Errorf("duration = %d, 期望 ttl 映射为 3600", body.Duration)
	}
	if body.Reason != "恶意扫描" || body.Scope != "site" || body.Site != "shop" {
		t.Errorf("请求体字段不符: %+v", body)
	}
}

// TestBanIPMissingIP 验证缺 ip 时返回 isError 结果而非访问控制台。
func TestBanIPMissingIP(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("POST /api/bans", func(w http.ResponseWriter, _ *http.Request) {
		okJSON(w, `{"id":1}`)
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	text, isErr := toolResult(t, s.call(t, "ban-2", "tools/call", `{"name":"waf_ban_ip","arguments":{}}`))
	if !isErr {
		t.Fatalf("缺 ip 应返回 isError 结果")
	}
	if !strings.Contains(text, "ip") {
		t.Errorf("错误信息应提到 ip,实际: %s", text)
	}
	if fc.size() != 0 {
		t.Errorf("参数缺失时不应访问控制台,实际收到 %d 个请求", fc.size())
	}
}

// TestCallUnbanIP 验证解封工具:DELETE 路径拼装与字符串 ID 兼容。
func TestCallUnbanIP(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/bans/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			failJSON(w, http.StatusMethodNotAllowed, 405, "方法不支持")
			return
		}
		okJSON(w, `{"ok":true}`)
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	// 字符串形式的 ID 也要能用
	text, isErr := toolResult(t, s.call(t, "ub-1", "tools/call",
		`{"name":"waf_unban_ip","arguments":{"id":"9"}}`))
	if isErr {
		t.Fatalf("unban_ip 不应报错: %s", text)
	}
	if !strings.Contains(text, `"ok":true`) {
		t.Errorf("text 应包含解封结果,实际: %s", text)
	}

	req := fc.captured(t, 0)
	if req.Method != "DELETE" || req.Path != pathBans+"/9" {
		t.Errorf("请求 = %s %s, 期望 DELETE %s/9", req.Method, req.Path, pathBans)
	}
}

// TestCallEventDetail 验证事件详情工具的路径拼装与非法 ID 校验。
func TestCallEventDetail(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/events/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/events/")
		okJSON(w, fmt.Sprintf(`{"id":%s,"action":"block","score":100}`, id))
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	text, isErr := toolResult(t, s.call(t, "ed-1", "tools/call",
		`{"name":"waf_event_detail","arguments":{"id":5}}`))
	if isErr {
		t.Fatalf("event_detail 不应报错: %s", text)
	}
	if !strings.Contains(text, `"id":5`) || !strings.Contains(text, `"action":"block"`) {
		t.Errorf("text 应为事件详情,实际: %s", text)
	}
	if req := fc.captured(t, 0); req.Path != pathEvents+"/5" {
		t.Errorf("请求路径 = %s, 期望 %s/5", req.Path, pathEvents)
	}

	// 非法 ID(负数)应报参数错误,不打控制台
	_, isErr = toolResult(t, s.call(t, "ed-2", "tools/call",
		`{"name":"waf_event_detail","arguments":{"id":-1}}`))
	if !isErr {
		t.Fatalf("非法 id 应返回 isError 结果")
	}
}

// TestCallTimeseries 验证时间序列工具的 hours 档位映射与缺省值。
func TestCallTimeseries(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/dashboard/timeseries", func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, fmt.Sprintf(`{"range":%q,"bucket":900,"points":[]}`, r.URL.Query().Get("range")))
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	cases := []struct {
		args string
		want string // 期望发出的 range 档位
	}{
		{`{"name":"waf_timeseries","arguments":{}}`, "24h"},          // 缺省 24 小时
		{`{"name":"waf_timeseries","arguments":{"hours":0}}`, "24h"}, // 0 视为缺省
		{`{"name":"waf_timeseries","arguments":{"hours":2}}`, "6h"},  // 就近取档
		{`{"name":"waf_timeseries","arguments":{"hours":24}}`, "24h"},
		{`{"name":"waf_timeseries","arguments":{"hours":48}}`, "7d"},
		{`{"name":"waf_timeseries","arguments":{"hours":200}}`, "30d"},
	}
	for i, tc := range cases {
		text, isErr := toolResult(t, s.call(t, fmt.Sprintf("ts-%d", i), "tools/call", tc.args))
		if isErr {
			t.Fatalf("用例 %d 不应报错: %s", i, text)
		}
		if !strings.Contains(text, `"range":"`+tc.want+`"`) {
			t.Errorf("用例 %d 控制台应收到 range=%s,实际 data: %s", i, tc.want, text)
		}
		if req := fc.captured(t, i); !strings.Contains(req.Query, "range="+tc.want) {
			t.Errorf("用例 %d 查询串 = %s, 期望包含 range=%s", i, req.Query, tc.want)
		}
	}
}

// TestCallEventsFilters 验证事件列表工具的分页与过滤参数透传。
func TestCallEventsFilters(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, _ *http.Request) {
		okJSON(w, `{"records":[],"current":1,"size":20,"total":0}`)
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	_, isErr := toolResult(t, s.call(t, "ev-1", "tools/call",
		`{"name":"waf_events","arguments":{"action":"block","site":"shop","keyword":"login"}}`))
	if isErr {
		t.Fatalf("waf_events 不应报错")
	}
	req := fc.captured(t, 0)
	for _, want := range []string{"current=1", "size=20", "action=block", "site=shop", "keyword=login"} {
		if !strings.Contains(req.Query, want) {
			t.Errorf("查询串应包含 %s,实际: %s", want, req.Query)
		}
	}
}

// TestCallBansPaging 验证封禁列表工具固定第一页 50 条。
func TestCallBansPaging(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/bans", func(w http.ResponseWriter, _ *http.Request) {
		okJSON(w, `{"records":[],"current":1,"size":50,"total":0}`)
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	_, isErr := toolResult(t, s.call(t, "bl-1", "tools/call", `{"name":"waf_bans"}`))
	if isErr {
		t.Fatalf("waf_bans 不应报错")
	}
	req := fc.captured(t, 0)
	if !strings.Contains(req.Query, "current=1") || !strings.Contains(req.Query, "size=50") {
		t.Errorf("查询串应包含 current=1 与 size=50,实际: %s", req.Query)
	}
}

// TestCallRuleTest 验证规则测试工具的请求体透传与 method 归一化。
func TestCallRuleTest(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("POST /api/rules/test", func(w http.ResponseWriter, _ *http.Request) {
		okJSON(w, `{"score":100,"action":"block","ruleIds":["930100"],"matches":[]}`)
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	text, isErr := toolResult(t, s.call(t, "rt-1", "tools/call",
		`{"name":"waf_rule_test","arguments":{"method":"get","path":"/a.php","query":"id=1","headers":{"User-Agent":"sqlmap"}}}`))
	if isErr {
		t.Fatalf("rule_test 不应报错: %s", text)
	}
	if !strings.Contains(text, `"score":100`) {
		t.Errorf("text 应包含测试结果,实际: %s", text)
	}

	var body ruleTestBody
	if err := json.Unmarshal([]byte(fc.captured(t, 0).Body), &body); err != nil {
		t.Fatalf("请求体不是合法 JSON: %v", err)
	}
	if body.Method != "GET" {
		t.Errorf("method = %q, 期望归一化为 GET", body.Method)
	}
	if body.Path != "/a.php" || body.Query != "id=1" {
		t.Errorf("path/query 透传不符: %+v", body)
	}
	if body.Headers["User-Agent"] != "sqlmap" {
		t.Errorf("headers 透传不符: %+v", body.Headers)
	}
}

// TestCallSitesAndSettings 验证无参工具的路径与解包。
func TestCallSitesAndSettings(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/sites", func(w http.ResponseWriter, _ *http.Request) {
		okJSON(w, `{"records":[{"name":"shop","upstream":"http://127.0.0.1:8080"}],"total":1}`)
	})
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, _ *http.Request) {
		okJSON(w, `{"settings":{"mode":"block"},"engine":{"rules":42},"demo":false}`)
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	text, isErr := toolResult(t, s.call(t, "st-1", "tools/call", `{"name":"waf_sites"}`))
	if isErr || !strings.Contains(text, `"name":"shop"`) {
		t.Fatalf("waf_sites 结果不符: isErr=%v text=%s", isErr, text)
	}
	text, isErr = toolResult(t, s.call(t, "st-2", "tools/call", `{"name":"waf_settings"}`))
	if isErr || !strings.Contains(text, `"mode":"block"`) {
		t.Fatalf("waf_settings 结果不符: isErr=%v text=%s", isErr, text)
	}
	if req := fc.captured(t, 0); req.Path != pathSites {
		t.Errorf("请求路径 = %s, 期望 %s", req.Path, pathSites)
	}
	if req := fc.captured(t, 1); req.Path != pathSettings {
		t.Errorf("请求路径 = %s, 期望 %s", req.Path, pathSettings)
	}
}

// ===== 错误传播与配置测试 =====

// TestToolErrorPropagation 验证控制台业务错误与 HTTP 状态错误都转成 isError 结果。
func TestToolErrorPropagation(t *testing.T) {
	fc, mux := newFakeConsole(t)
	// 业务错误:HTTP 200 但 code!=200,错误信息应带业务码与提示
	mux.HandleFunc("POST /api/bans", func(w http.ResponseWriter, _ *http.Request) {
		failJSON(w, http.StatusOK, 500, "数据库操作失败")
	})
	// HTTP 状态错误:401 未授权
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, _ *http.Request) {
		failJSON(w, http.StatusUnauthorized, 401, "登录已过期")
	})
	s := startSession(t, fc.srv.URL, "tok-1")

	// 控制台返回 401:错误信息应带状态码与提示
	text, isErr := toolResult(t, s.call(t, "err-1", "tools/call", `{"name":"waf_events"}`))
	if !isErr {
		t.Fatalf("401 应转为 isError 结果")
	}
	if !strings.Contains(text, "401") || !strings.Contains(text, "登录已过期") {
		t.Errorf("错误信息应包含状态码与提示,实际: %s", text)
	}

	// 控制台返回业务码 500:错误信息应带业务码与提示
	text, isErr = toolResult(t, s.call(t, "err-3", "tools/call",
		`{"name":"waf_ban_ip","arguments":{"ip":"1.2.3.4"}}`))
	if !isErr {
		t.Fatalf("业务码 500 应转为 isError 结果")
	}
	if !strings.Contains(text, "500") || !strings.Contains(text, "数据库操作失败") {
		t.Errorf("错误信息应包含业务码与提示,实际: %s", text)
	}

	// 控制台路径 404(mux 未注册 /api/rules):同样转 isError
	text, isErr = toolResult(t, s.call(t, "err-2", "tools/call", `{"name":"waf_rules"}`))
	if !isErr {
		t.Fatalf("404 应转为 isError 结果")
	}
	if !strings.Contains(text, "404") {
		t.Errorf("错误信息应包含状态码,实际: %s", text)
	}
}

// TestTokenMissing 验证未配置 GOWAF_TOKEN 时 initialize/tools/list 可用、
// tools/call 全部返回带配置指引的 isError 结果,且不发起控制台请求。
func TestTokenMissing(t *testing.T) {
	fc, mux := newFakeConsole(t)
	mux.HandleFunc("/api/dashboard/overview", func(w http.ResponseWriter, _ *http.Request) {
		okJSON(w, `{"total":1}`)
	})
	s := startSession(t, fc.srv.URL, "")

	// initialize 仍可用
	assertNoError(t, s.call(t, "i-1", "initialize", `{}`))

	// tools/list 仍可用
	resp := s.call(t, "tl-2", "tools/list", "")
	assertNoError(t, resp)

	// tools/call 返回 isError 与配置指引,不访问控制台
	text, isErr := toolResult(t, s.call(t, "c-1", "tools/call", `{"name":"waf_overview"}`))
	if !isErr {
		t.Fatalf("未配置令牌时 tools/call 应返回 isError 结果")
	}
	if !strings.Contains(text, EnvToken) || !strings.Contains(text, EnvURL) {
		t.Errorf("错误信息应包含环境变量指引,实际: %s", text)
	}
	if fc.size() != 0 {
		t.Errorf("未配置令牌时不应访问控制台,实际收到 %d 个请求", fc.size())
	}
}
