// Package response 统一控制面响应格式。
//
// 契约以 Frontend 前端为准：{code, msg, data}，code=200 表示成功。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务码，与 HTTP 状态码保持一致，避免前端"业务码 401 但 HTTP 200"的单边登出 bug。
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

// Body 统一响应体。
type Body struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

// OK 成功。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: CodeOK, Msg: "ok", Data: data})
}

// Fail 失败，httpStatus 与 code 通常一致。
func Fail(c *gin.Context, httpStatus, code int, msg string) {
	c.AbortWithStatusJSON(httpStatus, Body{Code: code, Msg: msg})
}

// BadRequest 参数错误。
func BadRequest(c *gin.Context, msg string) {
	Fail(c, http.StatusBadRequest, CodeBadRequest, msg)
}

// Unauthorized 未登录。
func Unauthorized(c *gin.Context, msg string) {
	Fail(c, http.StatusUnauthorized, CodeUnauthorized, msg)
}

// Forbidden 无权限。
func Forbidden(c *gin.Context, msg string) {
	Fail(c, http.StatusForbidden, CodeForbidden, msg)
}

// NotFound 资源不存在。
func NotFound(c *gin.Context, msg string) {
	Fail(c, http.StatusNotFound, CodeNotFound, msg)
}

// Internal 内部错误。
func Internal(c *gin.Context, msg string) {
	Fail(c, http.StatusInternalServerError, CodeInternal, msg)
}

// Page 分页响应，字段名对齐前端 Api.Common.PaginatedResponse。
type Page[T any] struct {
	Records []T   `json:"records"`
	Current int   `json:"current"`
	Size    int   `json:"size"`
	Total   int64 `json:"total"`
}

// NewPage 构造分页响应（records 为 nil 时返回空数组而非 null）。
func NewPage[T any](list []T, current, size int, total int64) Page[T] {
	if list == nil {
		list = []T{}
	}
	return Page[T]{Records: list, Current: current, Size: size, Total: total}
}
