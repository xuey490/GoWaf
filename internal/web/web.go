// Package web 把前端构建产物嵌进二进制。
//
// 注意：go:embed 不允许 ../，所以构建时要把 Frontend/dist 同步到 internal/web/dist
// （Makefile 的 sync-web 目标负责）。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Sub 返回 dist 子文件系统。
func Sub() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}

// Available 判断是否嵌入了真实前端产物（而不是占位页）。
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
