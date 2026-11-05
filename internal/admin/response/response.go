// Package response 统一控制面响应格式。
//
// 契约以 Frontend 前端为准：{code, msg, data}，code=200 表示成功。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务码常量：取值与 HTTP 状态码保持一致，避免前端出现"业务码 401 但 HTTP 200"
// 的单边登出 bug；CodeDemoReadOnly（40301）是唯一的扩展码，表示演示环境只读。
const (
	CodeOK           = 200
	CodeBadRequest   = 400
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeDemoReadOnly = 40301
	CodeNotFound     = 404
	CodeConflict     = 409
	CodeTooMany      = 429
	CodeInternal     = 500
)

// Body 统一响应体，控制面所有接口都按 {code, msg, data} 结构返回。
type Body struct {
	Code int    `json:"code"`           // 业务码，200 表示成功，其余取值见业务码常量。
	Msg  string `json:"msg"`            // 提示信息，成功时固定为 "ok"。
	Data any    `json:"data,omitempty"` // 业务数据，为空时整个字段省略。
}

// OK 返回成功响应（HTTP 200，业务码 200）。
// 参数 c：Gin 上下文；data：业务数据，可为 nil。
// 返回：无（响应直接写出）。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: CodeOK, Msg: "ok", Data: data})
}

// Fail 返回失败响应并中止后续 handler 链，httpStatus 与 code 通常保持一致。
// 参数 c：Gin 上下文；httpStatus：HTTP 状态码；code：业务码；msg：错误提示。
// 返回：无。
func Fail(c *gin.Context, httpStatus, code int, msg string) {
	c.AbortWithStatusJSON(httpStatus, Body{Code: code, Msg: msg})
}

// BadRequest 返回参数错误响应（HTTP 400 / 业务码 400）。
// 参数 c：Gin 上下文；msg：错误提示。
// 返回：无。
func BadRequest(c *gin.Context, msg string) {
	Fail(c, http.StatusBadRequest, CodeBadRequest, msg)
}

// Unauthorized 返回未登录/令牌失效响应（HTTP 401 / 业务码 401），
// 前端据此清理本地登录态并跳转登录页。
// 参数 c：Gin 上下文；msg：错误提示。
// 返回：无。
func Unauthorized(c *gin.Context, msg string) {
	Fail(c, http.StatusUnauthorized, CodeUnauthorized, msg)
}

// Forbidden 返回无权限/被拒绝响应（HTTP 403 / 业务码 403）。
// 参数 c：Gin 上下文；msg：错误提示。
// 返回：无。
func Forbidden(c *gin.Context, msg string) {
	Fail(c, http.StatusForbidden, CodeForbidden, msg)
}

// NotFound 返回资源不存在响应（HTTP 404 / 业务码 404）。
// 参数 c：Gin 上下文；msg：错误提示。
// 返回：无。
func NotFound(c *gin.Context, msg string) {
	Fail(c, http.StatusNotFound, CodeNotFound, msg)
}

// Internal 返回服务内部错误响应（HTTP 500 / 业务码 500）。
// 参数 c：Gin 上下文；msg：错误提示。
// 返回：无。
func Internal(c *gin.Context, msg string) {
	Fail(c, http.StatusInternalServerError, CodeInternal, msg)
}

// Page 分页响应体，字段名对齐前端 Api.Common.PaginatedResponse 契约。
type Page[T any] struct {
	Records []T   `json:"records"` // 当前页数据列表。
	Current int   `json:"current"` // 当前页码。
	Size    int   `json:"size"`    // 每页条数。
	Total   int64 `json:"total"`   // 总记录数。
}

// NewPage 构造分页响应体；list 为 nil 时替换为空切片，保证前端拿到 [] 而非 null。
// 参数 list：当前页数据；current：当前页码；size：每页条数；total：总记录数。
// 返回：填充好的分页响应体。
func NewPage[T any](list []T, current, size int, total int64) Page[T] {
	if list == nil {
		list = []T{}
	}
	return Page[T]{Records: list, Current: current, Size: size, Total: total}
}
