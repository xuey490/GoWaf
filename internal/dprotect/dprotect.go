// Package dprotect 实现动态防护：HTML 文本节点每次访问动态加密，浏览器端 JS 解密还原。
//
// 与雷池"动态防护"同思路的最小可行实现，对抗目标是"不执行 JS 的采集器"：
//
//  1. 服务端按请求生成一次性密钥，把 HTML 里的可见文本加密成乱码；
//  2. 页面尾部注入解密脚本，浏览器执行后还原原文；
//  3. curl / requests 这类不跑 JS 的客户端拿到的永远是密文。
//
// 密钥传递与对抗强度（诚实声明）：密钥本身必须随页面下发（浏览器要能解密），
// 所以本包只做"提高爬取成本"——密钥按字符码偏移混淆存放，脚本内联。配合
// dprotect.challenge 开启人机校验门禁后，未通过 PoW 的客户端连加密页都拿不到，
// 两层叠加才是完整的"动态防护 + 人机校验"语义。
//
// 实现细节：
//   - 只加密文本节点；script/style/textarea/noscript 的内容一律不碰，防止改坏
//     脚本与表单预填值；属性值（href/value 等）同样不动；
//   - 密文放在 <span class="wafx"> 里，解密脚本用 textContent 还原并摘掉壳节点；
//   - 经 golang.org/x/net/html 重排过的 DOM 结构可能与源码有细微差异（自动补全
//     标签等），对绝大多数页面无感；极端畸形页面解析失败时按原文透传（fail-open）。
package dprotect

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"gowaf/internal/metrics"
)

// decryptJS 内嵌的浏览器端解密脚本。
//
//go:embed decrypt.js
var decryptJS string

// wafxClass 占位节点的 class 与查询选择器，前后端约定一致。
const wafxClass = "wafx"

// keyLen 会话密钥长度（字节）。RC4 一次 KSA 吃掉全部密钥，32 字节足够。
const keyLen = 32

// keyObfuscateXor 密钥混淆的异或常量：decrypt.js 里用同一常量做逆运算。
const keyObfuscateXor = 0x5A

// skipTags 不做文本加密的元素：脚本样式不能动（动了必坏页面），
// textarea 的文本是表单预填值，noscript 是禁 JS 时的降级提示。
var skipTags = map[string]bool{
	"script": true, "style": true, "textarea": true, "noscript": true,
}

// Settings 动态防护配置，由 Reload 时从设置表编译，数据面只读。
type Settings struct {
	Enabled      bool     // 总开关，默认关闭（动态防护会改动页面 DOM，交给管理员按站点开启）
	MaxBytes     int      // 参与加密的页面体积上限（字节），超过直接透传
	ExcludePaths []string // 不加密的路径前缀（如 /api、/static）
	Challenge    bool     // 人机校验门禁：未通过 PoW 的访客先完成校验再拿正文
}

// DefaultMaxBytes 默认加密上限：2MB。HTML 页面极少超过 2MB，
// 超限的基本是大导出页面，直接透传不做 DOM 重排。
const DefaultMaxBytes = 2 << 20

// DefaultSettings 返回默认配置（关闭）。
// 无参数；返回 Settings：首次启用前的安全默认值。
func DefaultSettings() Settings {
	return Settings{Enabled: false, MaxBytes: DefaultMaxBytes}
}

// Applies 判断本请求是否应做动态加密。
// 参数 s：当前设置；参数 method：HTTP 方法；参数 path：请求路径。
// 返回 bool：未启用返回 false；非 GET/HEAD 返回 false（写请求的响应一般是
// JSON，没有加密意义）；命中排除前缀返回 false；其余 true。
func Applies(s *Settings, method, path string) bool {
	if s == nil || !s.Enabled {
		return false
	}
	if method != "GET" && method != "HEAD" {
		return false
	}
	for _, p := range s.ExcludePaths {
		if p != "" && strings.HasPrefix(path, p) {
			return false
		}
	}
	return true
}

// Manager 动态防护管理器：持有页面密钥派生的根密钥，无状态、并发安全。
type Manager struct {
	secret []byte // 根密钥：来自 JWT 密钥文件内容，与挑战通行证同源但不复用数值
}

// New 创建动态防护管理器。
// 参数 secret：根密钥（>=16 字节）。
// 返回 (*Manager, error)：密钥过短返回错误，否则返回就绪的管理器。
func New(secret []byte) (*Manager, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("动态防护根密钥过短（%d 字节，至少 16）", len(secret))
	}
	return &Manager{secret: append([]byte(nil), secret...)}, nil
}

// requestKey 为单次请求派生 32 字节密钥。
// 参数 nonce：16 字节随机数（随响应下发，用于密钥轮换）。
// 返回 []byte：HMAC-SHA256(secret, nonce) 摘要。
func (m *Manager) requestKey(nonce []byte) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(nonce)
	return mac.Sum(nil)
}

// obfuscateKey 把密钥转成注入页面的混淆字节数组。
// 参数 key：请求密钥。
// 返回 []byte：按 (byte ^ 0x5A ^ idx&7) 混淆的字节数组，与 decrypt.js 的逆运算对应。
func obfuscateKey(key []byte) []byte {
	out := make([]byte, len(key))
	for idx, b := range key {
		out[idx] = b ^ keyObfuscateXor ^ byte(idx&7)
	}
	return out
}

// Protect 加密一份 HTML 响应的可见文本并注入解密脚本。
// 参数 body：明文 HTML 响应体。
// 返回 ([]byte, error)：加密后的页面；解析失败或无正文时返回原文与错误，
// 调用方应按 fail-open 语义透传原文。
func (m *Manager) Protect(body []byte) ([]byte, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return body, fmt.Errorf("空响应体")
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return body, fmt.Errorf("HTML 解析失败: %w", err)
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return body, fmt.Errorf("生成随机数失败: %w", err)
	}
	key := m.requestKey(nonce)
	changed := 0
	encryptTextNodes(doc, key, &changed)
	if changed == 0 {
		// 没有任何可加密文本（如纯图片页）：不必注入脚本，原文返回即可
		return body, nil
	}

	// 注入解密脚本：参数先于脚本本体（脚本自执行时读 window.__wafxK）。
	// 密钥混淆数组用 JSON 数字数组承载，避免引号/转义问题；
	// nonce 是纯十六进制，直接字符串字面量即可。
	var params bytes.Buffer
	params.WriteString("window.__wafxK=[")
	for idx, b := range obfuscateKey(key) {
		if idx > 0 {
			params.WriteByte(',')
		}
		params.WriteString(fmt.Sprint(b))
	}
	params.WriteString("];window.__wafxN=\"")
	params.WriteString(fmt.Sprintf("%x", nonce))
	params.WriteString("\";")

	if err := injectScript(doc, params.String()+decryptJS); err != nil {
		return body, fmt.Errorf("注入解密脚本失败: %w", err)
	}

	var out bytes.Buffer
	out.Grow(len(body) + len(decryptJS) + keyLen*2)
	if err := html.Render(&out, doc); err != nil {
		return body, fmt.Errorf("HTML 渲染失败: %w", err)
	}
	metrics.DProtectPages.Inc()
	return out.Bytes(), nil
}

// encryptTextNodes 遍历 DOM，把加密范围内的文本节点替换成 <span class="wafx"> 密文。
// 参数 n：当前节点；参数 key：请求密钥；参数 changed：命中计数（就地累加）。
// 返回值无。
func encryptTextNodes(n *html.Node, key []byte, changed *int) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling // 先记住后继：c 可能被替换摘除
		switch c.Type {
		case html.ElementNode:
			if !skipTags[strings.ToLower(c.Data)] {
				encryptTextNodes(c, key, changed)
			}
		case html.TextNode:
			if len(strings.TrimSpace(c.Data)) == 0 {
				break // 纯空白布局文本，加密没有意义
			}
			replaceWithCipher(c, key, changed)
		}
		c = next
	}
}

// replaceWithCipher 把一个文本节点替换为占位 span。
// 参数 node：文本节点；参数 key：请求密钥；参数 changed：命中计数。
// 返回值无；任何一步失败都保留原节点不动（fail-open，宁可不防护也不改坏页面）。
func replaceWithCipher(node *html.Node, key []byte, changed *int) {
	cipher := cryptXOR([]byte(node.Data), key)
	b64 := base64.StdEncoding.EncodeToString(cipher)

	span := &html.Node{Type: html.ElementNode, Data: "span"}
	span.Attr = []html.Attribute{{Key: "class", Val: wafxClass}}
	span.AppendChild(&html.Node{Type: html.TextNode, Data: b64})
	if node.Parent == nil {
		return
	}
	node.Parent.InsertBefore(span, node)
	node.Parent.RemoveChild(node)
	*changed++
}

// injectScript 在 </body> 前插入解密脚本；找不到 body 时挂到文档根。
// 参数 doc：文档节点；参数 code：脚本完整代码（参数 + 解密器）。
// 返回 error：文档结构完全异常时返回错误。
func injectScript(doc *html.Node, code string) error {
	var body *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if body != nil {
			return
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "body") {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
			if body != nil {
				return
			}
		}
	}
	find(doc)

	script := &html.Node{Type: html.ElementNode, Data: "script"}
	script.AppendChild(&html.Node{Type: html.TextNode, Data: code})
	if body == nil {
		if doc.LastChild == nil {
			return fmt.Errorf("文档无内容")
		}
		doc.AppendChild(script)
		return nil
	}
	body.AppendChild(script)
	return nil
}
