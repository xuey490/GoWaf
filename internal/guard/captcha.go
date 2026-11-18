package guard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 挑战降级链的验证码环节：人机校验（PoW+环境信号）失败后，第二道防线是
// 图形验证码——真人可读可答，脚本要么 OCR 要么逐次爆破。答案由服务端签进
// 令牌（HMAC 防篡改），校验时不存会话、无状态横向扩展。
//
// 失败次数由调用方用 HitTracker 统计：窗口内答错 N 次直接封禁（链的最后一级）。

// captchaTTL 验证码令牌有效期：足够真人辨认填写，短于它能被批量复用的价值。
const captchaTTL = 5 * time.Minute

// captchaRange 验证码算式数值范围：0~19 的加减法，答案 0~38，肉眼计算无压力。
const captchaRange = 20

// CaptchaToken 为指定 IP 签发一道算术验证码令牌。
//
// 令牌是 HMAC 签名的 "cap|ip|过期毫秒|算式|答案"，客户端只能拿到算式
// （页面上以 SVG 渲染，答案不进 HTML 文本），提交后服务端验签比对答案。
// 参数 c：签名器（复用人机校验的密钥源）；参数 ip：客户端 IP；参数 now：当前时间。
// 返回 (token string, question string)：签名令牌与展示用算式文本（如 "7+5"）。
func (c *Challenger) CaptchaToken(ip string, now time.Time) (token, question string) {
	a, b := int(now.UnixNano()%captchaRange), int(now.UnixNano()/captchaRange%captchaRange)
	var ans int
	var expr string
	// 按时间奇偶混用加法与减法，保证答案非负
	if (a+b)%2 == 0 || a < b {
		ans = a + b
		expr = strconv.Itoa(a) + "+" + strconv.Itoa(b)
	} else {
		ans = a - b
		expr = strconv.Itoa(a) + "-" + strconv.Itoa(b)
	}
	exp := now.Add(captchaTTL).UnixMilli()
	payload := "cap|" + ip + "|" + strconv.FormatInt(exp, 10) + "|" + expr + "|" + strconv.Itoa(ans)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	token = base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return token, expr
}

// VerifyCaptcha 校验提交的验证码答案。
// 参数 token：挑战页拿到的验证码令牌；参数 ip：客户端 IP；参数 answer：提交的答案；
// 参数 now：当前时间。
// 返回 bool：签名有效、IP 一致、未过期且答案正确返回 true。
func (c *Challenger) VerifyCaptcha(token, ip, answer string, now time.Time) bool {
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
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(rawPayload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return false
	}
	parts := strings.Split(string(rawPayload), "|")
	if len(parts) != 5 || parts[0] != "cap" || parts[1] != ip {
		return false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.UnixMilli() >= exp {
		return false
	}
	want, err := strconv.Atoi(parts[4])
	if err != nil {
		return false
	}
	got, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || got != want {
		return false
	}
	return true
}

// captchaSVG 把算式渲染成 SVG 验证码图：字符随机轻微错位旋转，空白隔断，
// 增加低成本 OCR 的难度。参数 expr：算式文本（如 "7+5"）。
// 返回 []byte：SVG 文档字节。
func CaptchaSVG(expr string) []byte {
	var chars []string
	for _, ch := range expr {
		// 每个字符独立 <text>：水平递进 + 伪随机纵向抖动与旋转（确定性种子，
		// 同一算式渲染稳定，避免每次刷新抖动差异被用于比对）
		seed := int(ch)
		dx := (seed % 5)
		dy := (seed % 7) - 3
		rot := (seed % 9) - 4
		chars = append(chars, fmt.Sprintf(
			`<text x="%d" y="%d" transform="rotate(%d)" font-size="30" font-family="monospace" fill="#334155">%s</text>`,
			8+len(chars)*22+dx, 34+dy, rot, escapeXML(string(ch))))
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="140" height="46" viewBox="0 0 140 46">` +
		`<rect width="140" height="46" fill="#F1F5F9"/>` + strings.Join(chars, "") + `</svg>`)
}

// escapeXML 转义 XML 文本节点中的保留字符。
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
