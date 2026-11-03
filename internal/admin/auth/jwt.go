// Package auth 负责 JWT 签发校验与口令散列。
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

// 令牌用途。
const (
	PurposeAccess  = "access"
	PurposeRefresh = "refresh"
)

// Claims 自定义声明。
type Claims struct {
	UserID   uint   `json:"uid"`
	UserName string `json:"name"`
	Role     string `json:"role"`
	Purpose  string `json:"pur"`
	jwt.RegisteredClaims
}

// Manager 签发与校验令牌。
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
}

// NewManager 加载或生成密钥文件。
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

// loadOrCreateSecret 复用已有密钥，缺失时生成 32 字节随机值并落盘 0600。
func loadOrCreateSecret(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("未配置 jwt_secret_file")
	}
	if raw, err := os.ReadFile(path); err == nil {
		decoded, derr := hex.DecodeString(string(raw))
		if derr == nil && len(decoded) >= 32 {
			return decoded, nil
		}
		if len(raw) >= 32 {
			return raw, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("创建密钥目录: %w", err)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(buf)), 0o600); err != nil {
		return nil, fmt.Errorf("写入密钥文件: %w", err)
	}
	return buf, nil
}

// Secret 返回密钥（供 CSRF / Challenge 复用）。
func (m *Manager) Secret() []byte { return m.secret }

// AccessTTL 返回访问令牌有效期。
func (m *Manager) AccessTTL() time.Duration { return m.accessTTL }

// RefreshTTL 返回刷新令牌有效期。
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

// IssueAccess 签发访问令牌。
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

// IssueRefresh 签发刷新令牌，返回 token 与 jti（jti 用于落库吊销）。
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

// Parse 校验并解析令牌。
func (m *Manager) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名算法: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("令牌无效")
	}
	return claims, nil
}

// HashPassword bcrypt cost=12。
func HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), 12)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword 校验口令。
func VerifyPassword(hash, pwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd)) == nil
}

// randomID 生成 16 字节随机 hex。
func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
