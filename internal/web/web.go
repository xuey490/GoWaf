// Package web 把前端构建产物嵌进二进制。
//
// 注意：go:embed 不允许 ../，所以构建时要把 Frontend/dist 同步到 internal/web/dist
// （Makefile 的 sync-web 目标负责）。
package web

import (
	"embed"
	"io/fs"
)

// embedded 内嵌的前端静态文件根目录，构建期由 go:embed all:dist 打进二进制。
// all: 前缀确保以 _ 或 . 开头的文件（如前端框架的 _next 目录）也会被打包。
//
//go:embed all:dist
var embedded embed.FS

// Sub 返回 dist 子文件系统。
// 参数：无。
// 返回 (fs.FS, error)：以 dist 为根的只读文件系统，供管理后台路由直接挂载；
// 子路径在编译期固定，正常情况下不会返回错误。
func Sub() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}

// Available 判断是否嵌入了真实前端产物（而不是占位页）。
// 参数：无。
// 返回 bool：dist 目录可读且包含 index.html 时为 true；目录缺失、
// 不可读或只有占位文件时为 false。
func Available() bool {
	entries, err := embedded.ReadDir("dist")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() == "index.html" {
			return true
		}
	}
	return false
}
