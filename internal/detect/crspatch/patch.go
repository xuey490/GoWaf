// Package crspatch 为内嵌 CRS v4.30.0 提供补丁层。
//
// 背景：coraza-coreruleset/crs/v4 v4.30.0 打包时遗漏了
// @owasp_crs/web-shells-php.data（上游 coreruleset v4.30.0 tag 中该文件存在，
// 属打包仓库的 bug），导致 RESPONSE-955-WEB-SHELLS.conf 编译失败、整个 CRS
// 加载报错。该文件包含数百条真实 webshell 特征串，直接落盘会被 Windows
// Defender 当作威胁查杀，因此这里以 base64 形式入库、运行时在内存解码，
// 通过一个极小的 fs.FS 把文件"补"进 Coraza 的根文件系统（mergefs 中排第一，
// 优先级最高，不会遮蔽 crs/v4 提供的任何真实文件）。
package crspatch

import (
	"bytes"
	"embed"
	"encoding/base64"
	"io"
	"io/fs"
	"strings"
	"time"
)

// b64File 是内嵌的 webshells.b64：web-shells-php.data 的 base64 编码文本。
//
//go:embed webshells.b64
var b64File embed.FS

// b64Data 是 web-shells-php.data 的 base64 编码（来源：coreruleset v4.30.0）。
var b64Data []byte

// init 在包加载时读取 webshells.b64 并解码为原始数据，进程启动即就绪。
// 失败直接 panic：补丁数据损坏属于发布事故，带病启动没有意义。
func init() {
	raw, err := b64File.ReadFile("webshells.b64")
	if err != nil {
		panic("crspatch: 读取 webshells.b64 失败: " + err.Error())
	}
	// 去掉换行与空白，兼容带换行排版的 base64 文本
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, string(raw))
	decoded, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		panic("crspatch: 解码 webshells.b64 失败: " + err.Error())
	}
	b64Data = decoded
}

// FS 提供打包仓库缺失的 CRS 数据文件。用法：
//
//	coraza.NewWAF(coraza.NewWAFConfig().
//	    WithRootFS(mergefs.Merge(crspatch.FS, crs.FS, corazaconf.FS)))
//
// mergefs 的第一个 FS 优先，本包只提供 crs/v4 缺失的文件名，因此不会
// 覆盖任何上游真实文件。路径按两种形态都注册：裸文件名（Coraza 相对
// conf 解析时的形态）与 @owasp_crs/ 前缀形态。
var FS fs.FS = patchFS{}

// patchFileName 是要补进 CRS 虚拟文件系统的数据文件名。
const patchFileName = "web-shells-php.data"

// patchName 命中任意以该文件名结尾的请求路径。
// 参数 name：待检查的文件路径。返回 bool：是否请求的是补丁数据文件。
func patchName(name string) bool {
	return strings.HasSuffix(name, patchFileName)
}

// patchFS 是极小的只读 fs.FS，只提供 web-shells-php.data 一个文件。
type patchFS struct{}

// Open 实现 fs.FS：请求补丁文件时返回内存中的解码数据，其余路径一律 ErrNotExist。
// 参数 name：请求的文件路径。返回 fs.File：只读文件；返回 error：非补丁文件时非空。
func (patchFS) Open(name string) (fs.File, error) {
	if !patchName(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &patchFile{r: bytes.NewReader(b64Data)}, nil
}

// patchFile 是一个最小的只读 fs.File 实现。
type patchFile struct {
	r     *bytes.Reader
	score bool // 预留字段，当前未使用
}

// Read 实现 fs.File 读取：从内存解码数据拷贝到 p。
// 参数 p：目标缓冲。返回 n：读取字节数；err：读取错误。
func (f *patchFile) Read(p []byte) (int, error) { return f.r.Read(p) }

// Close 实现 fs.File：无底层资源需释放，恒返回 nil。
func (f *patchFile) Close() error { return nil }

// Stat 实现 fs.File：返回固定的文件元信息。
func (f *patchFile) Stat() (fs.FileInfo, error) { return patchInfo{}, nil }

// ReadAt 实现 io.ReaderAt：支持按偏移读取（Coraza 解析数据文件依赖）。
// 参数 p：目标缓冲；off：读取偏移。返回 n：读取字节数；err：读取错误。
func (f *patchFile) ReadAt(p []byte, off int64) (int, error) {
	return f.r.ReadAt(p, off)
}

// 编译期断言：patchFile 同时实现了 io.ReaderAt。
var _ io.ReaderAt = (*patchFile)(nil)

// patchInfo 固定返回文件元信息。
type patchInfo struct{}

// Name 实现 fs.FileInfo：返回补丁数据文件名。
func (patchInfo) Name() string { return patchFileName }

// Size 实现 fs.FileInfo：返回解码后数据长度。
func (patchInfo) Size() int64 { return int64(len(b64Data)) }

// Mode 实现 fs.FileInfo：返回只读权限 0444。
func (patchInfo) Mode() fs.FileMode {
	return fs.FileMode(0o444)
}

// ModTime 实现 fs.FileInfo：无真实修改时间，返回零值。
func (patchInfo) ModTime() time.Time { return time.Time{} }

// IsDir 实现 fs.FileInfo：恒为 false（普通文件）。
func (patchInfo) IsDir() bool { return false }

// Sys 实现 fs.FileInfo：恒为 nil。
func (patchInfo) Sys() any { return nil }
