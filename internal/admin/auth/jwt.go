// Package auth 负责管理后台的 JWT 令牌签发与校验，以及口令的 bcrypt 散列。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"gowaf/internal/store/model"
)

// PurposeAccess/PurposeRefresh 是令牌用途常量，写入 Claims.Purpose，
// 用于区分访问令牌与刷新令牌：刷新令牌只能用于换取新令牌，
// 不能直接调用业务接口（Auth 中间件据此拒绝）。
const (
	PurposeAccess  = "access"
	PurposeRefresh = "refresh"
)

// Claims 自定义 JWT 声明：在标准声明之外附带用户身份与令牌用途。
type Claims struct {
	UserID               uint   `json:"uid"`  // 用户 ID，对应 model.User.ID。
	UserName             string `json:"name"` // 用户名，同时作为标准声明 Subject。
	Role                 string `json:"role"` // 用户角色，供权限中间件使用。
	Purpose              string `json:"pur"`  // 令牌用途：PurposeAccess 或 PurposeRefresh。
	jwt.RegisteredClaims        // 内嵌标准声明（iss、sub、iat、nbf、exp、jti 等）。
}

// Manager JWT 令牌管理器：持有 HMAC 签名密钥与两类令牌的有效期，负责签发与校验。
type Manager struct {
	secret     []byte        // HMAC-SHA256 签名密钥，来自密钥文件。
	accessTTL  time.Duration // 访问令牌有效期。
	refreshTTL time.Duration // 刷新令牌有效期。
	issuer     string        // 标准声明 iss，签发时写入，校验时强制匹配。
}

// NewManager 创建令牌管理器：加载或生成密钥文件，并对非法的 TTL 参数取默认值。
// 参数 secretFile：密钥文件路径，文件缺失时自动生成并落盘。
// 参数 accessTTL：访问令牌有效期，<=0 时默认 2 小时。
// 参数 refreshTTL：刷新令牌有效期，<=0 时默认 7 天。
// 返回：令牌管理器；密钥加载失败时返回错误。
func NewManager(secretFile string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	secret, err := loadOrCreateSecret(secretFile)
	if err != nil {
		return nil, err
	}
	if accessTTL <= 0 {
		accessTTL = 2 * time.Hour
	}
	if refreshTTL <= 0 {
		refreshTTL = 7 * 24 * time.Hour
	}
	return &Manager{
		secret:     secret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		issuer:     "gowaf",
	}, nil
}

// loadOrCreateSecret 读取已有密钥；文件缺失或密钥过短时生成 32 字节随机值，
// 以 hex 编码写入文件并设置 0600 权限。
// 参数 path：密钥文件路径，为空时直接报错。
// 返回：密钥字节串；目录创建或文件写入失败时返回错误。
func loadOrCreateSecret(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("未配置 jwt_secret_file")
	}
	if raw, err := os.ReadFile(path); err == nil {
		// 文件内容为 hex 编码时先解码，解码成功且达到 32 字节即视为有效密钥
		decoded, derr := hex.DecodeString(string(raw))
		if derr == nil && len(decoded) >= 32 {
			return decoded, nil
		}
		// 非 hex 内容（或解码失败）时退回原始字节，同样要求至少 32 字节
		if len(raw) >= 32 {
			return raw, nil
		}
	}
	// 密钥不存在或过短，重新生成
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("创建密钥目录: %w", err)
	}
	// 生成 32 字节（256 位）随机密钥
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(buf)), 0o600); err != nil {
		return nil, fmt.Errorf("写入密钥文件: %w", err)
	}
	return buf, nil
}

// Secret 返回 HMAC 签名密钥，供 CSRF、Challenge 等模块复用同一密钥。
// 参数：无。
// 返回：密钥字节串。
func (m *Manager) Secret() []byte { return m.secret }

// AccessTTL 返回访问令牌的有效期。
// 参数：无。
// 返回：访问令牌 TTL。
func (m *Manager) AccessTTL() time.Duration { return m.accessTTL }

// RefreshTTL 返回刷新令牌的有效期。
// 参数：无。
// 返回：刷新令牌 TTL。
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

// IssueAccess 为指定用户签发访问令牌（HS256 签名）。
// 参数 u：目标用户，取其 ID、用户名与角色写入声明。
// 返回：签名的 JWT 字符串；签名失败时返回错误。
// 说明：NotBefore 回拨 30 秒以容忍签发方与校验方的时钟偏移，jti 为随机 ID。
func (m *Manager) IssueAccess(u *model.User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   u.ID,
		UserName: u.UserName,
		Role:     u.Role,
		Purpose:  PurposeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   u.UserName,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
			ID:        randomID(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// IssueRefresh 为指定用户签发刷新令牌（HS256 签名）。
// 参数 u：目标用户，取其 ID、用户名与角色写入声明。
// 返回：token 为签名的 JWT 字符串；jti 为令牌唯一 ID（服务端落库用于吊销）；签名失败时返回错误。
func (m *Manager) IssueRefresh(u *model.User) (token, jti string, err error) {
	now := time.Now()
	jti = randomID()
	claims := Claims{
		UserID:   u.ID,
		UserName: u.UserName,
		Role:     u.Role,
		Purpose:  PurposeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   u.UserName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshTTL)),
			ID:        jti,
		},
	}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	return token, jti, err
}

// Parse 校验并解析 JWT 令牌：校验签名、签发者（iss）与过期时间（exp）。
// 参数 raw：原始 JWT 字符串。
// 返回：解析出的自定义声明；令牌无效、签名算法不符或已过期时返回错误。
func (m *Manager) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	// WithIssuer 强制校验 iss；WithExpirationRequired 强制要求 exp 声明存在
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		// 仅接受 HMAC 家族签名算法，防止算法混淆攻击（如 alg=none、RS256 混用）
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名算法: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	// 库校验通过后再显式确认有效性，双保险
	if !tok.Valid {
		return nil, errors.New("令牌无效")
	}
	return claims, nil
}

// HashPassword 使用 bcrypt（cost=12）对口令做散列。
// 参数 pwd：明文口令。
// 返回：bcrypt 散列字符串；生成失败时返回错误。
func HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), 12)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword 校验明文口令与 bcrypt 散列是否匹配。
// 参数 hash：库中保存的 bcrypt 散列；pwd：用户输入的明文口令。
// 返回：匹配返回 true，否则返回 false。
func VerifyPassword(hash, pwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd)) == nil
}

// randomID 生成 16 字节随机数的 hex 字符串，用作令牌 jti。
// 参数：无。
// 返回：32 字符 hex 串；随机源失败时退化为纳秒时间戳字符串。
func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
