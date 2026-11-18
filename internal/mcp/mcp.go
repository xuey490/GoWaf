// Package mcp 实现内置的 MCP(Model Context Protocol)server,让 AI 客户端
// (如 Claude)通过 MCP 管理面板数据。
//
// 运行方式:`gowaf mcp` 子命令(main.go 接线)调用 Run,在 stdin/stdout 上以
// JSON-RPC 2.0 提供服务。分帧采用「每行一个 JSON 对象」——这是 MCP stdio
// 传输的约定格式,实现简单且无需 Content-Length 状态机,故不做 LSP 式分帧。
// stdout 只承载协议帧,诊断日志一律写 stderr。
//
// 数据来源是本机控制台 REST API(internal/admin/router.go 注册的路由,默认
// http://127.0.0.1:8899),认证使用控制台 API Token,以
// Authorization: Bearer <token> 方式携带,由环境变量 GOWAF_TOKEN 配置,
// GOWAF_URL 可覆盖控制台地址。令牌缺失时 initialize 与 tools/list 仍可用,
// 所有 tools/call 返回带配置指引的 isError 结果。
//
// 已实现方法:initialize、notifications/initialized(通知,忽略)、tools/list、
// tools/call、ping;未知通知一律忽略,未知请求方法返回 -32601。
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// MCP 协议常量与 JSON-RPC 2.0 标准错误码。
const (
	jsonrpcVersion  = "2.0"        // 本包所有协议帧的 jsonrpc 字段取值
	protocolVersion = "2024-11-05" // initialize 返回的 MCP 协议版本
	serverName      = "gowaf-mcp"  // initialize 返回的 serverInfo.name

	errCodeParse          = -32700 // 请求不是合法 JSON
	errCodeInvalidReq     = -32600 // 非法请求(缺 method、jsonrpc 版本错误等)
	errCodeMethodNotFound = -32601 // 方法不存在
	errCodeInvalidParams  = -32602 // 参数非法(缺 name、未知工具等)
	errCodeInternal       = -32603 // 服务内部错误
)

// JSON-RPC 方法名常量。
const (
	methodInitialize  = "initialize"
	methodInitialized = "notifications/initialized"
	methodToolsList   = "tools/list"
	methodToolsCall   = "tools/call"
	methodPing        = "ping"
)

// scanBufSize 是按行读取请求的单行上限(4 MiB):Scanner 默认 64 KiB,
// 装不下大 arguments 的请求行,必须显式放大。
const scanBufSize = 4 << 20

// rawIDNull 解析失败时响应使用的空 ID(JSON null,此时无法得知请求 ID)。
var rawIDNull = json.RawMessage("null")

// rpcRequest 是 JSON-RPC 2.0 请求帧;ID 缺省(原始 JSON 无 id 字段)即为通知。
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"` // 缺省或 null 表示通知,不应回复
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"` // 可缺省
}

// rpcError 是 JSON-RPC 2.0 错误对象。
type rpcError struct {
	Code    int    `json:"code"`           // 标准错误码,取值见 errCode* 常量
	Message string `json:"message"`        // 人类可读的错误描述
	Data    any    `json:"data,omitempty"` // 附加调试信息,可缺省
}

// rpcResponse 是 JSON-RPC 2.0 响应帧:成功带 result,失败带 error,二者不同时出现。
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// textContent 是 tools/call 返回的文本内容块。
type textContent struct {
	Type string `json:"type"` // 固定为 "text"
	Text string `json:"text"` // 内容文本,此处为 data 部分的 JSON 字符串或错误说明
}

// callToolResult 是 tools/call 的返回结构。
type callToolResult struct {
	Content []textContent `json:"content"`           // 内容块列表,当前只有 text 一种
	IsError bool          `json:"isError,omitempty"` // true 表示工具执行失败,text 为错误说明
}

// capabilities 描述本 server 支持的能力,当前只声明 tools。
type capabilities struct {
	Tools struct{} `json:"tools"` // 空对象表示支持 tools/list 与 tools/call,无子能力开关
}

// serverInfo 是 initialize 返回的服务器信息。
type serverInfo struct {
	Name    string `json:"name"`    // 固定 "gowaf-mcp"
	Version string `json:"version"` // 随主程序传入的版本号
}

// initializeResult 是 initialize 方法的返回结构。
type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"` // 本 server 支持的 MCP 协议版本
	Capabilities    capabilities `json:"capabilities"`    // {"tools":{}}
	ServerInfo      serverInfo   `json:"serverInfo"`
}

// toolsListResult 是 tools/list 的返回结构。
type toolsListResult struct {
	Tools []toolWire `json:"tools"`
}

// callParams 是 tools/call 的入参。
type callParams struct {
	Name      string          `json:"name"`      // 工具名
	Arguments json.RawMessage `json:"arguments"` // 工具入参对象,可缺省
}

// mcpServer 承载一次 MCP 会话:协议分发、工具注册表与控制台客户端。
type mcpServer struct {
	version string         // 主程序版本号,回填到 serverInfo.version
	client  *consoleClient // 控制台 REST 客户端(令牌可能未配置)
	out     io.Writer      // 协议输出流(stdout)
	errLog  io.Writer      // 诊断日志输出流(stderr),nil 表示丢弃
}

// Run 启动 MCP server:从 stdin 按行读取 JSON-RPC 请求,把响应写回 stdout,
// 阻塞直到 stdin 关闭(EOF)。供 main.go 的 `gowaf mcp` 子命令调用。
// 控制台地址与令牌从环境变量 GOWAF_URL / GOWAF_TOKEN 读取:GOWAF_TOKEN 未设置
// 时 initialize 与 tools/list 可用,tools/call 全部返回带配置指引的 isError 结果。
// 参数 stdin:请求输入流;stdout:响应输出流(只写协议帧);version:主程序版本号。
// 返回:读流出错时返回错误;会话正常结束(stdin EOF)返回 nil。
func Run(stdin io.Reader, stdout io.Writer, version string) error {
	s := &mcpServer{
		version: version,
		client:  newConsoleClient(),
		out:     stdout,
		errLog:  os.Stderr,
	}
	if s.client.token == "" {
		s.logf("未配置 %s,tools/call 将返回配置指引(控制台地址: %s)", EnvToken, s.client.base)
	}
	return s.serve(stdin)
}

// serve 主循环:逐行解析请求并回写响应;空行与通知不产生响应。
// 参数 stdin:请求输入流。
// 返回:读流出错时返回错误;正常 EOF 返回 nil。
func (s *mcpServer) serve(stdin io.Reader) error {
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 0, 64*1024), scanBufSize)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue // 空行:直接跳过,不回复
		}
		if resp := s.handleLine(line); resp != nil {
			if err := s.write(resp); err != nil {
				return fmt.Errorf("写出响应失败: %w", err)
			}
		}
	}
	return sc.Err() // 正常 EOF 时为 nil
}

// write 把一帧响应编码为单行 JSON 写到输出流。
// 参数 resp:待写出的响应帧。
// 返回:编码或写出失败时返回错误。
func (s *mcpServer) write(resp *rpcResponse) error {
	buf, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	_, err = s.out.Write(buf)
	return err
}

// logf 写一条诊断日志到 stderr(永不写 stdout,避免污染协议通道)。
func (s *mcpServer) logf(format string, args ...any) {
	if s.errLog == nil {
		return
	}
	_, _ = fmt.Fprintf(s.errLog, "[mcp] "+format+"\n", args...)
}

// handleLine 处理一行请求文本,需要回复时返回响应帧;通知与解析失败按约定处理。
// 参数 line:去掉首尾空白的一行请求。
// 返回:应回复的响应帧;通知返回 nil。
func (s *mcpServer) handleLine(line []byte) *rpcResponse {
	// 先探测 JSON 合法性,再解到请求结构:两类失败分别对应 -32700 与 -32600
	var probe any
	if err := json.Unmarshal(line, &probe); err != nil {
		return &rpcResponse{
			JSONRPC: jsonrpcVersion, ID: rawIDNull,
			Error: &rpcError{Code: errCodeParse, Message: "请求不是合法 JSON: " + err.Error()},
		}
	}
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResp(rawIDNull, errCodeInvalidReq, "请求必须是 JSON-RPC 2.0 对象: "+err.Error())
	}
	// jsonrpc 字段缺失按 2.0 宽容处理,写了别的版本则明确拒绝
	if req.JSONRPC != "" && req.JSONRPC != jsonrpcVersion {
		return errorResp(req.ID, errCodeInvalidReq, "jsonrpc 版本必须是 2.0")
	}
	// 通知(无 id 或 id 为 null,含未知通知)一律不回复
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return nil
	}
	if req.Method == "" {
		return errorResp(req.ID, errCodeInvalidReq, "缺少 method 字段")
	}

	result, rpcErr := s.dispatch(req.Method, req.Params)
	if rpcErr != nil {
		return errorResp(req.ID, rpcErr.Code, rpcErr.Message)
	}
	return &rpcResponse{JSONRPC: jsonrpcVersion, ID: req.ID, Result: result}
}

// dispatch 按方法名分发;未知方法返回 -32601。
// 参数 method:JSON-RPC 方法名;params:原始入参(可缺省)。
// 返回:成功时为结果对象;失败时为错误对象(含标准错误码)。
func (s *mcpServer) dispatch(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case methodInitialize:
		return s.resultInitialize(), nil
	case methodPing:
		return struct{}{}, nil
	case methodToolsList:
		return toolsListResult{Tools: wireTools()}, nil
	case methodToolsCall:
		return s.callTool(params)
	case methodInitialized:
		// notifications/initialized 本是通知(正常不会走到这里),
		// 客户端若带 id 发送也按无内容成功处理,保持宽容。
		return struct{}{}, nil
	default:
		return nil, &rpcError{Code: errCodeMethodNotFound, Message: "未知方法: " + method}
	}
}

// resultInitialize 构造 initialize 的返回:协议版本、能力与服务信息。
// 该方法不校验令牌,让客户端在未配置时也能完成握手并看到工具清单。
// 返回:initializeResult。
func (s *mcpServer) resultInitialize() initializeResult {
	return initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    capabilities{},
		ServerInfo:      serverInfo{Name: serverName, Version: s.version},
	}
}

// callTool 执行 tools/call:按名查找工具并调用。工具执行失败(含未配置令牌)
// 转成 isError 结果,不用 JSON-RPC error——工具级失败属于正常结果,
// 客户端可以直接把 text 转述给用户;缺 name 或未知工具才算协议级参数错误。
// 参数 params:原始入参(name/arguments)。
// 返回:callToolResult;params 非法或工具不存在时返回 -32602 错误。
func (s *mcpServer) callTool(params json.RawMessage) (any, *rpcError) {
	var p callParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: errCodeInvalidParams, Message: "params 不是合法 JSON 对象: " + err.Error()}
		}
	}
	if p.Name == "" {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "缺少必填参数 name"}
	}
	tool := lookupTool(p.Name)
	if tool == nil {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "未知工具: " + p.Name}
	}
	if s.client.token == "" {
		return callToolResult{
			Content: []textContent{newText(toolTokenMissingHint())},
			IsError: true,
		}, nil
	}

	data, err := tool.Handler(context.Background(), s.client, p.Arguments)
	if err != nil {
		s.logf("工具 %s 执行失败: %v", p.Name, err)
		return callToolResult{Content: []textContent{newText(err.Error())}, IsError: true}, nil
	}
	return callToolResult{Content: []textContent{newText(rawText(data))}}, nil
}

// rawText 把控制台返回的 data 原文转为 text 内容;data 缺省时回传 "null",
// 保证 content.text 始终是合法 JSON 字符串。
func rawText(data json.RawMessage) string {
	if len(data) == 0 {
		return "null"
	}
	return string(data)
}

// toolTokenMissingHint 生成未配置令牌时的统一报错文案,写清配置方法。
// 返回:提示文本。
func toolTokenMissingHint() string {
	return "未配置控制台访问令牌:请设置环境变量 " + EnvToken + "=<控制台生成的 API Token>," +
		"可选设置 " + EnvURL + "(控制台地址,默认 " + defaultURL + "),然后重启本 MCP server。"
}

// newText 构造一个 text 内容块。
// 参数 text:文本内容。
// 返回:内容块。
func newText(text string) textContent {
	return textContent{Type: "text", Text: text}
}

// lookupTool 按名称查找工具注册表;注册表只有十余项,线性查找足够。
// 参数 name:工具名。
// 返回:命中的工具定义;未注册返回 nil。
func lookupTool(name string) *toolDef {
	for _, t := range toolRegistry {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// errorResp 构造一帧带错误的响应。
// 参数 id:请求 ID 原文;code:标准错误码;msg:错误描述。
// 返回:响应帧。
func errorResp(id json.RawMessage, code int, msg string) *rpcResponse {
	return &rpcResponse{
		JSONRPC: jsonrpcVersion, ID: id,
		Error: &rpcError{Code: code, Message: msg},
	}
}
