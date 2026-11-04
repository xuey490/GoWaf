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

//go:embed webshells.b64
var b64File embed.FS

// b64Data 是 web-shells-php.data 的 base64 编码（来源：coreruleset v4.30.0）。
var b64Data []byte

func init() {
	raw, err := b64File.ReadFile("webshells.b64")
	if err != nil {
		panic("crspatch: 读取 webshells.b64 失败: " + err.Error())
	}
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

const patchFileName = "web-shells-php.data"

// patchName 命中任意以该文件名结尾的请求路径。
func patchName(name string) bool {
	return strings.HasSuffix(name, patchFileName)
}

type patchFS struct{}

func (patchFS) Open(name string) (fs.File, error) {
	if !patchName(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &patchFile{r: bytes.NewReader(b64Data)}, nil
}

// patchFile 是一个最小的只读 fs.File 实现。
type patchFile struct {
	r     *bytes.Reader
	score bool // 是否已调用过 Stat
}

func (f *patchFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *patchFile) Close() error               { return nil }
func (f *patchFile) Stat() (fs.FileInfo, error) { return patchInfo{}, nil }
func (f *patchFile) ReadAt(p []byte, off int64) (int, error) {
	return f.r.ReadAt(p, off)
}

var _ io.ReaderAt = (*patchFile)(nil)

// patchInfo 固定返回文件元信息。
type patchInfo struct{}

func (patchInfo) Name() string { return patchFileName }
func (patchInfo) Size() int64  { return int64(len(b64Data)) }
func (patchInfo) Mode() fs.FileMode {
	return fs.FileMode(0o444)
}
func (patchInfo) ModTime() time.Time { return time.Time{} }
func (patchInfo) IsDir() bool        { return false }
func (patchInfo) Sys() any           { return nil }
