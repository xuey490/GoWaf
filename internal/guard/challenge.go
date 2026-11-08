package guard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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
