package edge

import (
	"html"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"gowaf/internal/guard"
	"gowaf/internal/metrics"
	"gowaf/internal/proxy"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// captchaPathPrefix 验证码挑战的路径前缀：验证码页面与校验端点都挂在这下面，
// 与策略认证（/.waf-auth/）同级，必须先于全部闸门放行——被门禁挡住的访客
// 靠它换取通行证。
const captchaPathPrefix = "/.waf-captcha"

// challengeChain 解析降级链配置。
// 参数 v：设置原文（逗号分隔，如 "pow,captcha,ban"）。
// 返回 (hasCaptcha bool, banOnFail bool)：链中是否包含验证码级与封禁级。
func challengeChain(v string) (hasCaptcha, banOnFail bool) {
	for _, p := range strings.Split(strings.ToLower(v), ",") {
		switch strings.TrimSpace(p) {
		case "captcha":
			hasCaptcha = true
		case "ban":
			banOnFail = true
		}
	}
	return hasCaptcha, banOnFail
}

// handleCaptcha 验证码页面与校验端点的统一入口。
// 路由：GET /.waf-captcha/（验证码挑战页）、GET /.waf-captcha/verify（答案提交）。
// 参数 w：响应写出器；参数 r：客户端请求；参数 ip：客户端 IP。
// 返回值无。
func (h *Handler) handleCaptcha(w http.ResponseWriter, r *http.Request, ip netip.Addr) {
	if r.URL.Path == captchaPathPrefix+"/verify" {
		h.verifyCaptcha(w, r, ip)
		return
	}
	h.serveCaptchaPage(w, r, ip, "")
}

// serveCaptchaPage 渲染验证码挑战页（算式 SVG + 表单）。
// 参数 errText：上次答错的原因提示（空串表示首次进入）。
// 返回值无。
func (h *Handler) serveCaptchaPage(w http.ResponseWriter, r *http.Request, ip netip.Addr, errText string) {
	token, question := h.rt.Chall.CaptchaToken(ip.String(), time.Now())
	svgB64 := base64SVG(guard.CaptchaSVG(question))
	to := r.URL.Query().Get("to")
	if to == "" || !strings.HasPrefix(to, "/") {
		to = "/"
	}

	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 状态码固定 200：本页面既是独立挑战页，也可能是 issuePass 在 fetch 中
	// 返回的降级内容（403 会触发挑战页 JS 的 reload 循环，永远看不到表单）。
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>安全验证</title>
<style>
body{font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#334155;
display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;text-align:center;
max-width:420px;box-shadow:0 4px 24px rgba(15,23,42,.06)}
h1{font-size:18px;margin:0 0 14px}
img{border-radius:6px;margin-bottom:14px}
input{font-size:18px;width:120px;text-align:center;padding:6px;border:1px solid #CBD5E1;border-radius:6px}
button{font-size:15px;padding:7px 22px;margin-left:10px;border:0;border-radius:6px;
background:#2563EB;color:#fff;cursor:pointer}
.err{color:#DC2626;font-size:13px;min-height:18px;margin-bottom:10px}
p{color:#64748B;font-size:13px;margin:12px 0 0}
</style></head><body><div class="box">
<h1>请完成安全验证</h1>
<div class="err">`+html.EscapeString(errText)+`</div>
<img src="data:image/svg+xml;base64,`+svgB64+`" alt="验证码">
<form method="GET" action="`+captchaPathPrefix+`/verify">
<input type="hidden" name="t" value="`+html.EscapeString(token)+`">
<input type="hidden" name="to" value="`+html.EscapeString(to)+`">
<input name="a" autocomplete="off" inputmode="numeric" placeholder="答案" required>
<button type="submit">提交</button>
</form>
<p>请输入图片中算式的计算结果。</p>
</div></body></html>`)
}

// verifyCaptcha 校验提交的答案：通过签发通行证跳回目标；答错计数，
// 达阈值且降级链含 ban 时封禁该 IP。
func (h *Handler) verifyCaptcha(w http.ResponseWriter, r *http.Request, ip netip.Addr) {
	token := r.URL.Query().Get("t")
	answer := r.URL.Query().Get("a")
	to := r.URL.Query().Get("to")
	if to == "" || !strings.HasPrefix(to, "/") {
		to = "/"
	}

	if !h.rt.Chall.VerifyCaptcha(token, ip.String(), answer, time.Now()) {
		_, reached := h.rt.CaptchaFail(ip.String())
		if reached {
			_, banOn := challengeChain(h.rt.Setting("guard.challenge.chain", "pow"))
			if banOn {
				h.banCaptchaAbuser(ip.String())
			}
		}
		metrics.CaptchaFailsTotal.Inc()
		h.serveCaptchaPage(w, r, ip, "答案不正确，请重试。")
		return
	}
	metrics.CaptchaPassedTotal.Inc()
	// 通过：签发与人机校验（PoW）同一张通行证（30 分钟有效，绑定 IP）
	passToken := h.rt.Chall.Sign(ip.String(), time.Now())
	noStore(w)
	http.SetCookie(w, &http.Cookie{
		Name:     h.rt.Chall.CookieName(),
		Value:    passToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   1800,
	})
	http.Redirect(w, r, to, http.StatusFound)
}

// banCaptchaAbuser 验证码答错达阈值：写入内存封禁表并落库（与情报封禁同款式）。
func (h *Handler) banCaptchaAbuser(ip string) {
	ttl := 3600
	expires := store.NowMS() + int64(ttl)*1000
	h.rt.Bans.Ban(ip, "", expires)
	metrics.BannedGauge.Set(float64(h.rt.Bans.Size()))
	if err := h.rt.Store.Gorm().Create(&model.Ban{
		IP: ip, Reason: "验证码连续答错（挑战降级链）", Source: "captcha",
		Scope: "global", HitCount: 1,
		CreatedAt: store.NowMS(), ExpiresAt: expires,
	}).Error; err != nil {
		log.Printf("[edge] 写验证码封禁记录失败: %v", err)
	}
}

// serveWaitPage 等候室满员的「稍后重试」页：429 + Retry-After + 自动刷新，
// 浏览器按提示自动重试，等效于继续排队。
func (h *Handler) serveWaitPage(w http.ResponseWriter, r *http.Request, route *proxy.Route) {
	interval := h.rt.SettingInt("waitroom.interval", 3)
	if interval <= 0 {
		interval = 3
	}
	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Retry-After", strconv.Itoa(interval))
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = io.WriteString(w, `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta http-equiv="refresh" content="`+strconv.Itoa(interval)+`">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>排队等候中</title>
<style>
body{font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#334155;
display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;text-align:center;
max-width:420px;box-shadow:0 4px 24px rgba(15,23,42,.06)}
h1{font-size:18px;margin:0 0 10px}p{color:#64748B;font-size:14px;margin:0}
</style></head><body><div class="box">
<h1>当前访问人数较多</h1>
<p>您正在排队等候，页面将每 `+strconv.Itoa(interval)+` 秒自动重试。</p>
</div></body></html>`)
}

// base64SVG 把 SVG 编码成 data URI 用的 base64（标准编码）。
func base64SVG(svg []byte) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out strings.Builder
	for i := 0; i < len(svg); i += 3 {
		var b [3]byte
		n := copy(b[:], svg[i:])
		v := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
		out.WriteByte(chars[v>>18&63])
		out.WriteByte(chars[v>>12&63])
		if n > 1 {
			out.WriteByte(chars[v>>6&63])
		} else {
			out.WriteByte('=')
		}
		if n > 2 {
			out.WriteByte(chars[v&63])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
}
