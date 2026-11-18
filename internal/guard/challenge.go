package guard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// ChallengePoWBits 是自动校验的工作量证明难度（哈希前导零位数）。
//
// 16 位意味着平均要算 2^16 = 65536 次 SHA-256：浏览器里约 30-80 毫秒，
// 用户完全无感；但脚本每过一次校验都要付这个成本，把「一个请求换一张通行证」
// 的廉价绕过变成需要真算力，配合 30 分钟的通行证有效期，对 CC 类流量足够。
const ChallengePoWBits = 16

// challengeTokenTTL 是挑战令牌的有效期，只用于「页面拿到令牌 → 算完 PoW → 换通行证」，
// 所以必须短：令牌泄露出去也没法长期复用。
const challengeTokenTTL = 5 * time.Minute

// ChallengeToken 为指定 IP 生成一次挑战令牌。
//
// 令牌是 IP + 过期时间 + 难度的 HMAC 签名串，页面把它算进工作量证明里，
// 服务端校验「签名有效 + IP 一致 + 未过期 + 哈希前导零达标」才下发通行证。
// 这样即使有人手工构造 X-WAF-JS 头，没有合法令牌也算不出答案。
// 参数 ip：客户端 IP；参数 now：当前时间；参数 difficulty：前导零位数（<=0 时用默认值）。
// 返回 string：形如 base64(payload)+"."+base64(签名)，payload 为 "c|ip|过期毫秒|难度"。
func (c *Challenger) ChallengeToken(ip string, now time.Time, difficulty int) string {
	if difficulty <= 0 {
		difficulty = ChallengePoWBits
	}
	exp := now.Add(challengeTokenTTL).UnixMilli()
	payload := "c|" + ip + "|" + strconv.FormatInt(exp, 10) + "|" + strconv.Itoa(difficulty)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyPoW 校验挑战令牌与客户端提交的工作量证明。
// 参数 token：页面拿到的挑战令牌；参数 ip：当前请求的客户端 IP；
// 参数 counter：客户端找到的随机数；参数 now：当前时间。
// 返回 bool：签名有效、IP 一致、未过期且 sha256(token|counter) 前导零达标时为 true。
func (c *Challenger) VerifyPoW(token, ip string, counter uint64, now time.Time) bool {
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
	if len(parts) != 4 || parts[0] != "c" || parts[1] != ip {
		return false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.UnixMilli() >= exp {
		return false
	}
	difficulty, err := strconv.Atoi(parts[3])
	if err != nil || difficulty < 0 || difficulty > 32 {
		return false
	}
	return LeadingZeroBits(token, counter) >= difficulty
}

// PowIssuedAt 从挑战令牌解析签发时间（过期时间 - 令牌 TTL）。
// 供「最小耗时」校验使用：真实浏览器从拿到挑战页到算完 PoW 换通行证，
// 需要经历 JS 启动 + 平均 6.5 万次哈希（约 0.3 秒），而 Go/Python 脚本
// 解 16 位 PoW 只要微秒级——耗时过短的换证请求即脚本特征。
// 参数 token：挑战令牌原文。
// 返回 (int64, bool)：签发时间毫秒时间戳；令牌结构非法时返回 false。
func PowIssuedAt(token string) (int64, bool) {
	dot := strings.LastIndexByte(token, '.')
	if dot <= 0 {
		return 0, false
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return 0, false
	}
	parts := strings.Split(string(rawPayload), "|")
	if len(parts) != 4 || parts[0] != "c" {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, false
	}
	return exp - challengeTokenTTL.Milliseconds(), true
}

// EnvSignal 是挑战页 JS 上报的浏览器环境信号（JSON base64 传输）。
// 客户端数据不可信任，这里做的是「结构合理性」校验：真实浏览器必然能
// 填出全部字段且取值在合理区间；headless 自动化要么 webdriver 为 true
// 要么缺字段，脚本伪造全套字段需要逐字段逆向挑战页 JS——抬高成本是目的。
type EnvSignal struct {
	WD  *bool   `json:"wd"`  // navigator.webdriver：无头自动化标志，必须为 false
	HC  *int    `json:"hc"`  // hardwareConcurrency：CPU 核数
	SC  *string `json:"sc"`  // 屏幕 "宽x高"
	TZ  *string `json:"tz"`  // IANA 时区
	WGL *string `json:"wgl"` // WebGL 厂商/渲染器
	LN  string  `json:"ln"`  // navigator.languages（可选）
	PL  *int    `json:"pl"`  // navigator.plugins 数量（可选）
	PT  float64 `json:"pt"`  // performance.now() 页面耗时毫秒（可选）
}

// CheckEnvSignal 校验环境信号的结构合理性。
// 参数 raw：挑战页 JS 上报的 base64(JSON) 串；空串直接不通过。
// 返回 bool：全部必填字段存在、取值在合理区间且 webdriver 非真时为 true。
func CheckEnvSignal(raw string) bool {
	if raw == "" {
		return false
	}
	// 兼容标准/URL-safe 两种 base64，缺 padding 也接受（JS btoa 输出标准形）
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(raw)
	}
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(raw)
	}
	if err != nil {
		return false
	}
	var e EnvSignal
	if err := json.Unmarshal(decoded, &e); err != nil {
		return false
	}
	// webdriver：无头浏览器/自动化驱动必为 true
	if e.WD == nil || *e.WD {
		return false
	}
	// CPU 核数：真实设备 1~512，缺失或 0 说明 JS 被裁剪（jsdom 等）
	if e.HC == nil || *e.HC < 1 || *e.HC > 512 {
		return false
	}
	// 屏幕分辨率：宽高都在 100~20000 之间
	if e.SC == nil {
		return false
	}
	w, h, ok := parseScreenDims(*e.SC)
	if !ok || w < 100 || w > 20000 || h < 100 || h > 20000 {
		return false
	}
	// 时区：IANA 名长度合理
	if e.TZ == nil || len(*e.TZ) < 2 || len(*e.TZ) > 64 {
		return false
	}
	// WebGL 厂商/渲染器：真实环境必有值（禁用 WebGL 的浏览器会返回占位串）
	if e.WGL == nil || len(*e.WGL) < 2 || len(*e.WGL) > 160 {
		return false
	}
	// 页面耗时：从导航到算完 PoW 至少数百毫秒；负值或荒谬大值不通过
	if e.PT < 0 || e.PT > 600000 {
		return false
	}
	return true
}

// parseScreenDims 解析 "1920x1080" 形式的分辨率串。
// 返回 (int, int, bool)：宽、高与是否解析成功。
func parseScreenDims(s string) (int, int, bool) {
	i := strings.IndexByte(s, 'x')
	if i <= 0 || i == len(s)-1 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(s[:i])
	h, err2 := strconv.Atoi(s[i+1:])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return w, h, true
}

// LeadingZeroBits 计算 sha256(令牌|计数器) 的前导零位数，供服务端与前端共用同一算法。
// 前端用 crypto.subtle.digest("SHA-256", encode(token + "|" + counter)) 计算，
// 两边必须保持完全一致的拼接方式，否则校验永远不通过。
// 参数 token：挑战令牌原文；参数 counter：随机数。
// 返回 int：哈希值最高位起连续 0 的位数（0-256）。
func LeadingZeroBits(token string, counter uint64) int {
	sum := sha256.Sum256([]byte(token + "|" + strconv.FormatUint(counter, 10)))
	zeros := 0
	for _, b := range sum {
		if b == 0 {
			zeros += 8
			continue
		}
		for mask := byte(0x80); mask > 0 && b&mask == 0; mask >>= 1 {
			zeros++
		}
		break
	}
	return zeros
}
