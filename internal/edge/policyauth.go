package edge

// 精准访问控制的两个门禁动作：auth（页面密码/访问认证）与 rate_limit（自定义 CC）。
//
// 两个动作都在「缓存查找之前」的早期阶段执行（earlyPolicyVerdict，第 6.4 步），
// 否则命中缓存的资源会绕过门禁；完整求值（第 8 步）对它们免疫——策略引擎
// 编译期已保证门禁条件不依赖 body，早期阶段是它们唯一的执行点。
//
// auth 的两种模式：
//   - basic：HTTP Basic 认证，浏览器弹原生对话框，API 客户端天然可用；
//   - page：密码页，访客提交密码后签发 HMAC 签名的通行证 Cookie
//     （按策略独立命名 waf_auth_<policyID>，绑定 IP + 过期时间）。
//
// 密码只存 bcrypt 哈希（控制台创建时哈希，明文不落库）；通行证用
// runtime 派生的 policy-auth 密钥签名，与挑战/动态防护密钥同源不同值。

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gowaf/internal/metrics"
	"gowaf/internal/policy"
	"gowaf/internal/proxy"
)

// policyAuthPassPrefix 认证密码提交端点的 URL 前缀（数据面在站点路由后拦截）。
const policyAuthPassPrefix = "/.waf-auth/"

// policyAuthGate 认证门禁的判定与放行。
// 参数 m：命中的 auth 策略；参数 r：客户端请求；参数 ip：客户端 IP。
// 返回 bool：已持有有效通行证或 Basic 认证通过返回 true（继续后续管线）。
func (h *Handler) policyAuthGate(m *policy.Match, r *http.Request, ip netip.Addr) bool {
	cfg := m.Auth
	if cfg == nil {
		return true // 编译快照异常时不拦人：门禁失效按放行处理（fail-open）
	}
	if cfg.Mode == "basic" && basicAuthValid(cfg, r) {
		return true
	}
	// page 模式（以及 basic 模式没带头时——让浏览器也能用 Cookie 方式通过）
	if ck, err := r.Cookie(policy.PolicyAuthCookieName(m.PolicyID)); err == nil && ck.Value != "" {
		if h.verifyPolicyAuthToken(ck.Value, m.PolicyID, ip.String()) {
			return true
		}
	}
	return false
}

// basicAuthValid 校验 Basic Auth 凭据。
// 参数 cfg：认证配置；参数 r：客户端请求。
// 返回 bool：凭据格式合法且用户名+密码与配置中的 bcrypt 哈希匹配返回 true。
// 恒定时间比较由 bcrypt.CompareHashAndPassword 内部保证。
func basicAuthValid(cfg *policy.AuthConfig, r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	for _, u := range cfg.Users {
		if subtle.ConstantTimeCompare([]byte(user), []byte(u.User)) == 1 {
			return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(pass)) == nil
		}
	}
	// 用户名不存在也跑一次空比较，避免通过响应时间枚举用户名
	_ = bcrypt.CompareHashAndPassword(
		[]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0X0pQ2mU0Wjzzf4cS0fTvF0m1mS"), []byte(pass))
	return false
}

// signPolicyAuthToken 签发策略认证通行证。
// 参数 secret：policy-auth 密钥；参数 policyID：策略 ID；参数 ip：绑定的客户端 IP；
// 参数 exp：过期毫秒时间戳。
// 返回 string：base64(payload)+"."+base64(HMAC)。
func signPolicyAuthToken(secret []byte, policyID uint, ip string, exp int64) string {
	payload := fmt.Sprintf("a|%d|%s|%d", policyID, ip, exp)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyPolicyAuthToken 校验策略认证通行证：签名、策略 ID、IP 绑定与有效期。
// 参数 token：Cookie 值；参数 policyID：期望的策略 ID；参数 ip：当前客户端 IP。
// 返回 bool：全部通过返回 true。
func (h *Handler) verifyPolicyAuthToken(token string, policyID uint, ip string) bool {
	return verifyPolicyAuthTokenWith(h.rt.PolicyAuthSecret(), token, policyID, ip, time.Now())
}

// verifyPolicyAuthTokenWith 是通行证校验的纯函数实现（可独立测试）。
// 参数 secret：签名密钥；参数 token：Cookie 值；参数 policyID：期望策略 ID；
// 参数 ip：当前客户端 IP；参数 now：当前时间。
// 返回 bool：签名有效、策略一致、IP 一致且未过期时返回 true。
func verifyPolicyAuthTokenWith(secret []byte, token string, policyID uint, ip string, now time.Time) bool {
	dot := strings.LastIndexByte(token, '.')
	if dot <= 0 {
		return false
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(token[dot+1:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(rawPayload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return false
	}
	parts := strings.Split(string(rawPayload), "|")
	if len(parts) != 4 || parts[0] != "a" {
		return false
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || uint(id) != policyID || parts[2] != ip {
		return false
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	return err == nil && now.UnixMilli() < exp
}

// servePolicyAuth 认证未通过时的响应：basic 返回 401 挑战头，page 返回密码页。
// 参数 w/r/route/ip/reqID/start：标准请求上下文；参数 m：命中的策略。
// 返回值无：调用方在写出响应后直接结束请求；事件按 challenge 落库
// （看板的「人机校验」口径涵盖所有"先自证再放行"的交互，避免新动作撕裂统计）。
func (h *Handler) servePolicyAuth(w http.ResponseWriter, r *http.Request, route *proxy.Route,
	ip netip.Addr, reqID string, start time.Time, m *policy.Match) {
	cfg := m.Auth
	// 静态资源与 API 路径不下密码页：弹窗式保护只对页面导航有意义
	if cfg != nil && cfg.Mode == "basic" {
		realm := cfg.Realm
		if realm == "" {
			realm = "Restricted"
		}
		noStore(w)
		w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`", charset="UTF-8"`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, blockPage(http.StatusUnauthorized, "此区域需要访问认证。", reqID))
		res := policyResult(m)
		h.recordEvent(route, ip, reqID, start, r, http.StatusUnauthorized, "challenge", &res, 0, "", true)
		return
	}
	// page 模式：展示密码表单
	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	title := "访问验证"
	if cfg != nil && strings.TrimSpace(cfg.Title) != "" {
		title = html.EscapeString(cfg.Title)
	}
	_, _ = io.WriteString(w, `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>`+title+`</title>
<style>
body{font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;background:#F6F8FB;
color:#334155;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;text-align:center;
max-width:420px;box-shadow:0 4px 24px rgba(15,23,42,.06)}
h1{font-size:18px;margin:0 0 16px}
input[type=password]{width:100%;padding:10px 12px;border:1px solid #CBD5E1;border-radius:8px;
font-size:14px;box-sizing:border-box;margin-bottom:12px}
button{width:100%;padding:10px;background:#2563EB;color:#fff;border:0;border-radius:8px;
font-size:14px;cursor:pointer}
button:hover{background:#1D4ED8}
p{color:#64748B;font-size:13px;margin:10px 0 0}
.rid{margin-top:14px;font-size:12px;color:#94A3B8;font-family:ui-monospace,Menlo,monospace}
</style></head><body><div class="box">
<h1>`+title+`</h1>
<form method="post" action="`+policyAuthPassPrefix+strconv.FormatUint(uint64(m.PolicyID), 10)+`">
<input type="password" name="password" placeholder="请输入访问密码" autofocus required>
<button type="submit">进入</button>
</form>
<p>此页面受访问密码保护，请输入密码继续。</p>
<div class="rid">Request ID: `+html.EscapeString(reqID)+`</div>
</div></body></html>`)
	res := policyResult(m)
	h.recordEvent(route, ip, reqID, start, r, http.StatusForbidden, "challenge", &res, 0, "", true)
}

// handlePolicyAuthPass 处理密码提交（POST /.waf-auth/<policyID>）。
// 校验通过签发通行证 Cookie 并跳回 ?to= 指定的站内路径；失败重定向回来源页。
// 参数 w：响应写出器；参数 r：客户端请求；参数 route：站点路由（事件记录用，可为 nil）；
// 参数 ip：客户端 IP。
// 返回值无：响应直接写出，调用方结束请求。
func (h *Handler) handlePolicyAuthPass(w http.ResponseWriter, r *http.Request, route *proxy.Route, ip netip.Addr) {
	idStr := strings.TrimPrefix(r.URL.Path, policyAuthPassPrefix)
	id, err := strconv.ParseUint(idStr, 10, 64)
	cfg, ok := h.rt.PolicySet().AuthConfigByID(uint(id))
	if err != nil || !ok || cfg.Mode != "page" {
		http.Error(w, "无效的认证请求", http.StatusBadRequest)
		return
	}
	// 解析密码：application/x-www-form-urlencoded 的 password 字段
	if err := r.ParseForm(); err != nil || r.PostFormValue("password") == "" {
		http.Error(w, "缺少密码", http.StatusBadRequest)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(r.PostFormValue("password"))) != nil {
		// 失败不提示具体原因（密码错误/策略已改），回到来源页让用户重试
		time.Sleep(300 * time.Millisecond) // 轻微延迟抬高暴力尝试成本
		http.Redirect(w, r, safeRedirectTarget(r.URL.Query().Get("to")), http.StatusFound)
		return
	}
	ttl := time.Duration(cfg.TTLSec) * time.Second
	token := signPolicyAuthToken(h.rt.PolicyAuthSecret(), uint(id), ip.String(), time.Now().Add(ttl).UnixMilli())
	http.SetCookie(w, &http.Cookie{
		Name:     policy.PolicyAuthCookieName(uint(id)),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl / time.Second),
	})
	// 通行证下发响应绝不缓存（缓存住了别人就能复用这张 Cookie）
	noStore(w)
	http.Redirect(w, r, safeRedirectTarget(r.URL.Query().Get("to")), http.StatusFound)
}

// safeRedirectTarget 把跳回目标限制在站内路径，防止开放重定向。
// 参数 to：查询参数原文。
// 返回 string：以 / 开头的站内路径，非法时回退 "/"。
func safeRedirectTarget(to string) string {
	if to != "" && strings.HasPrefix(to, "/") && !strings.HasPrefix(to, "//") {
		return to
	}
	return "/"
}

// policyRateLimit 执行自定义 CC 规则：按「策略+IP」窗口计数，超限按配置处置。
// 参数 m：命中的 rate_limit 策略；参数 route/ip/reqID/start：标准上下文。
// 返回 bool：true 表示已超限并写出处置响应，调用方应结束请求；
// false 表示未超限，继续后续管线。
func (h *Handler) policyRateLimit(w http.ResponseWriter, r *http.Request, route *proxy.Route,
	ip netip.Addr, reqID string, start time.Time, m *policy.Match) bool {
	cfg := m.RateLimit
	if cfg == nil {
		return false
	}
	windowMS := int64(cfg.WindowSec) * 1000
	key := "pl" + strconv.FormatUint(uint64(m.PolicyID), 10) + "|" + ip.String()
	_, reached := h.rt.PolicyHits.Add(key, storeNowMS(), windowMS, cfg.Threshold)
	if !reached {
		return false
	}

	metrics.PolicyCCBlocks.Inc()
	res := policyResult(m)
	if cfg.OnExceed == "challenge" {
		if !h.challengePassed(w, r, ip) {
			h.serveChallenge(w, r, ip)
			h.recordEvent(route, ip, reqID, start, r, http.StatusForbidden, "challenge", &res, 0, "", true)
			return true
		}
		// 已持通行证：本次放行（通行证 30 分钟内不再被 CC 规则反复挑战）
		return false
	}
	metrics.BlockedTotal.WithLabelValues(route.SiteName, "cc").Inc()
	h.rejectWithBody(w, r, route, ip, reqID, start, "block", "cc",
		http.StatusTooManyRequests, "请求过于频繁（自定义限流规则）", &res)
	return true
}

// storeNowMS 是 store.NowMS 的本地别名（避免 edge 引入 store 包只为取时间戳）。
func storeNowMS() int64 { return time.Now().UnixMilli() }
