// mcp 包内部:控制台 REST 客户端。
//
// MCP 工具本身不做任何业务计算,全部数据来自本机控制台 REST API
// (internal/admin/router.go 注册的路由)。本文件负责:
//   - 从环境变量读取控制台地址与 API Token;
//   - 以 Authorization: Bearer <token> 方式携带令牌;
//   - 按 {code, msg, data} 统一包装(internal/admin/response)解包响应,
//     只把 data 部分交给工具层。
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// 环境变量与客户端默认值:控制台地址可选,API Token 必填。
const (
	EnvURL   = "GOWAF_URL"   // 控制台根地址环境变量,如 http://127.0.0.1:8899
	EnvToken = "GOWAF_TOKEN" // 控制台 API Token 环境变量,缺失时 tools/call 全部报错

	defaultURL    = "http://127.0.0.1:8899" // GOWAF_URL 未设置时使用的默认控制台地址
	httpTimeout   = 10 * time.Second        // 单次访问控制台的超时时间
	maxRespBytes  = 4 << 20                 // 控制台响应体读取上限(4 MiB),防异常响应撑爆内存
	errMsgSnippet = 200                     // 非 JSON 响应时错误信息里最多携带的原文长度
)

// apiCodeOK 控制台业务成功码,取值对齐 internal/admin/response.CodeOK。
const apiCodeOK = 200

// consoleClient 访问控制台 REST API 的轻量客户端。
//
// token 为空表示未配置令牌:此时不发起任何请求,由调用方(工具层)统一
// 返回带配置指引的错误,避免拿空令牌去打控制台白白产生 401。
type consoleClient struct {
	base  string       // 控制台根地址,末尾不带 "/"
	token string       // API Token,空串表示未配置
	hc    *http.Client // 带 10 秒超时的 HTTP 客户端
}

// newConsoleClient 从环境变量构造控制台客户端。
// 返回:就绪的客户端;GOWAF_URL 未设置时使用默认地址,GOWAF_TOKEN 允许为空
// (由调用方判空后给出配置提示)。
func newConsoleClient() *consoleClient {
	base := strings.TrimSpace(os.Getenv(EnvURL))
	if base == "" {
		base = defaultURL
	}
	return &consoleClient{
		base:  strings.TrimRight(base, "/"),
		token: strings.TrimSpace(os.Getenv(EnvToken)),
		hc:    &http.Client{Timeout: httpTimeout},
	}
}

// apiBody 控制台统一响应包装,字段对齐 internal/admin/response.Body。
type apiBody struct {
	Code int             `json:"code"`           // 业务码,200 表示成功
	Msg  string          `json:"msg"`            // 提示信息,成功时为 "ok"
	Data json.RawMessage `json:"data,omitempty"` // 业务数据原文,可能缺省
}

// get 发起 GET 请求并解包 data。
// 参数 ctx:取消/超时上下文;path:API 路径(以 / 开头);query:查询参数,可为 nil。
// 返回:data 部分的原始 JSON;HTTP 非 2xx、业务码非 200 或传输失败时返回描述性错误。
func (cc *consoleClient) get(ctx context.Context, path string, query url.Values) (json.RawMessage, error) {
	return cc.do(ctx, http.MethodGet, path, query, nil)
}

// postJSON 发起 POST 请求(JSON 体)并解包 data。
// 参数 body:任意可 JSON 序列化的请求体。
// 返回:同 get。
func (cc *consoleClient) postJSON(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return cc.do(ctx, http.MethodPost, path, nil, body)
}

// delete 发起 DELETE 请求并解包 data。
// 返回:同 get。
func (cc *consoleClient) delete(ctx context.Context, path string) (json.RawMessage, error) {
	return cc.do(ctx, http.MethodDelete, path, nil, nil)
}

// do 执行一次控制台请求并按 {code, msg, data} 解包。
// 参数 method:HTTP 方法;path:API 路径;query:查询参数;body:非 nil 时序列化为 JSON 请求体。
// 返回:成功时为 data 原文;失败时返回含 HTTP 状态码或业务码与提示信息的错误。
func (cc *consoleClient) do(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	u := cc.base + path
	if enc := query.Encode(); enc != "" {
		u += "?" + enc
	}
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("编码请求体失败: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cc.token != "" {
		req.Header.Set("Authorization", "Bearer "+cc.token)
	}

	resp, err := cc.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("访问控制台失败(%s %s): %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return nil, fmt.Errorf("读取控制台响应失败: %w", err)
	}

	// 非 JSON 响应按空包装处理,只报告状态码与原文片段,不让解包失败吞掉真实错误
	var payload apiBody
	_ = json.Unmarshal(raw, &payload)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("控制台返回 HTTP %d: %s", resp.StatusCode, orMsg(payload.Msg, raw))
	}
	if payload.Code != apiCodeOK {
		return nil, fmt.Errorf("控制台业务错误(码 %d): %s", payload.Code, orMsg(payload.Msg, raw))
	}
	return payload.Data, nil
}

// orMsg 挑选可展示的错误信息:优先业务 msg,空则截取响应原文片段。
// 参数 msg:解包出的业务提示;raw:响应原文。
// 返回:用于拼接进错误信息的人类可读文本。
func orMsg(msg string, raw []byte) string {
	msg = strings.TrimSpace(msg)
	if msg != "" {
		return msg
	}
	if len(raw) == 0 {
		return "(空响应)"
	}
	s := string(raw)
	if len(s) > errMsgSnippet {
		s = s[:errMsgSnippet] + "..."
	}
	return s
}
