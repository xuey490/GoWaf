// Package totp 实现控制台两步验证（RFC 6238 TOTP，HMAC-SHA1，30 秒步长）。
//
// 实现范围：密钥生成（Base32）、当前码与校验（±1 窗口容忍时钟漂移）、
// otpauth:// 绑定 URL（扫码器与 Google Authenticator 等兼容）。
// 不引入第三方依赖：HMAC-SHA1 与动态截断在标准库内即可完成。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// StepSize TOTP 步长：RFC 6238 推荐 30 秒。
const StepSize = 30 * time.Second

// SecretLen 密钥字节数：20 字节 = RFC 4226 推荐，Base32 后 32 字符。
const SecretLen = 20

// VerifyWindow 校验时允许的步长偏移：±1 步（±30 秒），容忍客户端时钟漂移。
const VerifyWindow = 1

// GenerateSecret 生成新的 TOTP 密钥。
// 返回 (string, error)：Base32 编码（无填充）密钥；随机源失败返回错误。
func GenerateSecret() (string, error) {
	buf := make([]byte, SecretLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// codeAt 计算指定时刻的动态码。
// 参数 secret：Base32 密钥；参数 t：计算时刻。
// 返回 string：6 位动态码；密钥非法返回空串。
func codeAt(secret string, t time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(
		strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil || len(key) == 0 {
		return ""
	}
	counter := uint64(t.Unix()) / uint64(StepSize/time.Second)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	// 动态截断（RFC 4226 5.3）：低 4 位做偏移，取 4 字节去掉最高位
	off := sum[len(sum)-1] & 0x0f
	v := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	code := v % 1000000
	return pad6(strconv.FormatUint(uint64(code), 10))
}

// zfill 左侧补零到 6 位（Go 无 %06d 的直接格式化场景由 strconv 处理）。
func pad6(code string) string {
	for len(code) < 6 {
		code = "0" + code
	}
	return code
}

// Code 计算当前时刻的动态码（展示/测试用）。
// 参数 secret：Base32 密钥。返回 string：6 位动态码；密钥非法返回空串。
func Code(secret string) string { return codeAt(secret, time.Now()) }

// Verify 常数时间校验动态码，容忍 ±VerifyWindow 步的时钟漂移。
// 参数 secret：Base32 密钥；参数 code：用户输入的 6 位码。
// 返回 bool：任一允许步长的码与输入一致返回 true。
func Verify(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	now := time.Now()
	for i := -VerifyWindow; i <= VerifyWindow; i++ {
		want := codeAt(secret, now.Add(time.Duration(i)*StepSize))
		if want == "" {
			return false
		}
		if hmac.Equal([]byte(want), []byte(pad6(code))) {
			return true
		}
	}
	return false
}

// ProvisionURL 生成 otpauth:// 绑定 URL，供扫码器生成二维码。
// 参数 issuer：站点标识（显示在验证器中的归属方）；参数 account：账号名；
// 参数 secret：Base32 密钥。
// 返回 string：otpauth URL。
func ProvisionURL(issuer, account, secret string) string {
	return (&url.URL{
		Scheme: "otpauth",
		Host:   "totp",
		Path:   "/" + issuer + ":" + account,
	}).String() + "?secret=" + secret + "&issuer=" + url.QueryEscape(issuer) +
		"&algorithm=SHA1&digits=6&period=" + strconv.Itoa(int(StepSize/time.Second))
}
