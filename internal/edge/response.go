package edge

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// newRequestID 生成 16 位十六进制请求 ID，用于日志/响应头/面板串联。
// 返回 string：8 字节随机数的十六进制编码（16 字符）；随机源失败时退化为纳秒
// 时间戳的 16 位十六进制表示，保证 ID 始终可用。
func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// blockPage 返回拦截页 HTML。不暴露具体规则，避免帮攻击者调 payload。
// 参数 status：HTTP 状态码（大字显示）；参数 text：提示文案（HTML 转义后输出）；
// 参数 reqID：请求 ID（供用户反馈误报时定位日志）。
// 返回 string：完整 HTML 页面文本，由调用方自行写出。
func blockPage(status int, text, reqID string) string {
	return `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + fmt.Sprint(status) + ` 请求被拦截</title>
<style>
body{font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;background:#F6F8FB;
color:#334155;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;text-align:center;
max-width:460px;box-shadow:0 4px 24px rgba(15,23,42,.06)}
.code{font-size:44px;font-weight:700;color:#DC2626;margin:0 0 8px}
h1{font-size:20px;margin:0 0 12px}
p{color:#64748B;margin:0 0 4px;font-size:14px}
.rid{margin-top:16px;font-size:12px;color:#94A3B8;font-family:ui-monospace,Menlo,monospace}
</style></head><body><div class="box">
<div class="code">` + fmt.Sprint(status) + `</div>
<h1>请求被安全策略拦截</h1>
<p>` + html.EscapeString(text) + `</p>
<p>如果这是误报，请把下方请求 ID 提供给管理员。</p>
<div class="rid">Request ID: ` + html.EscapeString(reqID) + `</div>
</div></body></html>`
}

// noStore 禁止浏览器与 CDN 缓存安全相关响应。
//
// 挑战页里带着目标地址，拦截页里带着请求 ID，两者都是按请求动态生成的。
// 站点前面挂了 CDN（Cloudflare 的 Cache Everything 规则会把 403 一起缓存）时，
// 少了这个头就会有「一次 403 被缓存、之后所有访客都拿到同一份挑战页」的问题，
// 表现是页面永远卡在校验中，或者正常用户莫名被拦。
// 参数 w：响应写出器，直接写入 Cache-Control/Pragma/Expires 三个响应头。
// 返回值无。
func noStore(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
}

// challengePassed 判断是否已通过人机校验。
// 参数 w：响应写出器（当前实现未使用，保留以备扩展）；参数 r：客户端请求；
// 参数 ip：客户端 IP（通行证与其绑定，换 IP 即失效）。
// 返回 bool：请求携带有效挑战 Cookie 且签名校验通过返回 true，否则 false。
func (h *Handler) challengePassed(w http.ResponseWriter, r *http.Request, ip netip.Addr) bool {
	ck, err := r.Cookie(h.rt.Chall.CookieName())
	if err != nil || ck.Value == "" {
		return false
	}
	return h.rt.Chall.Verify(ck.Value, ip.String(), time.Now())
}

// serveChallenge 返回挑战页。
//
// 原理：页面里用 JS 发一个带 X-WAF-JS 头的请求换通行证。
// 不执行 JS 的爬虫/扫描器拿不到 Cookie，下一步请求依旧被挑战。
// 参数 w：响应写出器；参数 r：客户端请求；参数 ip：客户端 IP。
// 返回值无：请求命中通行证路径或带 X-WAF-JS 头时直接下发通行证（issuePass），
// 否则写出一个会自动发起校验请求并跳回原地址的 HTML 页面。
func (h *Handler) serveChallenge(w http.ResponseWriter, r *http.Request, ip netip.Addr) {
	if r.URL.Path == h.rt.Chall.PassPath() {
		h.issuePass(w, r, ip)
		return
	}
	if r.Header.Get("X-WAF-JS") == "1" {
		h.issuePass(w, r, ip)
		return
	}

	target := html.EscapeString(r.URL.RequestURI())
	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>安全校验</title>
<style>
body{font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#334155;
display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;
text-align:center;max-width:420px}
h1{font-size:18px;margin:0 0 10px}p{color:#64748B;font-size:14px;margin:0}
.spin{width:28px;height:28px;border:3px solid #E5E9F2;border-top-color:#2563EB;
border-radius:50%;margin:0 auto 18px;animation:s .8s linear infinite}
@keyframes s{to{transform:rotate(360deg)}}
</style></head><body><div class="box">
<div class="spin"></div>
<h1>正在进行安全校验</h1>
<p>请稍候，页面会自动跳转。</p>
</div>
<script>
(function(){
  var to = "`+target+`";
  fetch("`+h.rt.Chall.PassPath()+`?to=" + encodeURIComponent(to), {
    headers: {"X-WAF-JS": "1"}, credentials: "same-origin", cache: "no-store"
  }).then(function(){ location.replace(to); })
    .catch(function(){ setTimeout(function(){ location.replace(to); }, 1500); });
})();
</script></body></html>`)
}

// issuePass 下发通行证并跳回原地址。
// 参数 w：响应写出器；参数 r：客户端请求；参数 ip：客户端 IP（签发时绑定，校验时比对）。
// 返回值无：签发通行证 Cookie（HttpOnly，30 分钟有效）后，JS 校验请求返回 204；
// 普通请求 302 跳回 ?to= 指定的站内路径（缺失或非站内路径时回退首页）。
func (h *Handler) issuePass(w http.ResponseWriter, r *http.Request, ip netip.Addr) {
	token := h.rt.Chall.Sign(ip.String(), time.Now())
	// 下发通行证的响应必须不缓存：缓存住了别人就能复用这张 Cookie。
	noStore(w)
	http.SetCookie(w, &http.Cookie{
		Name:     h.rt.Chall.CookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   1800,
	})
	if r.Header.Get("X-WAF-JS") == "1" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	to := r.URL.Query().Get("to")
	if to == "" || !strings.HasPrefix(to, "/") {
		to = "/"
	}
	http.Redirect(w, r, to, http.StatusFound)
}
