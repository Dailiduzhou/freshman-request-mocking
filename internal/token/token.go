package token

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 挑战阶段常量，写入 JWT 的 stage 声明。
const (
	StageChallenge = "challenge"
	StageReward    = "reward"
)

// Claims 是服务端签发的全部凭证类型。
// 初始 JWT：stage=challenge，jti 为会话键；
// 通关 JWT：stage=reward，sid 指向初始会话，jti 每次签发均不同。
type Claims struct {
	Stage string `json:"stage"`
	SID   string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// Manager 持有签名密钥。密钥仅保留在服务端内存中，
// 不出现在任何响应、日志或公开资源里。
type Manager struct {
	secret []byte
}

// NewFromEnv 优先读取 JWT_SECRET；未设置时使用随机密钥。
// 服务重启后内存会话本就失效，随机密钥不会带来额外问题。
func NewFromEnv() (*Manager, error) {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return New([]byte(s)), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("生成随机签名密钥失败: %w", err)
	}
	return New(buf), nil
}

// New 使用给定密钥构造 Manager，主要供测试使用。
func New(secret []byte) *Manager {
	return &Manager{secret: secret}
}

// NewJTI 生成随机会话标识。
func NewJTI() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// 密钥源不可用时无法继续签发，直接 panic 由上层恢复为 5xx。
		panic(fmt.Sprintf("生成 jti 失败: %v", err))
	}
	return hex.EncodeToString(buf)
}

// Issue 签发一张 JWT。算法固定为 HS256，不接受客户端选择算法。
func (m *Manager) Issue(stage, sid string, ttl time.Duration, now time.Time) (token, jti string, expires time.Time, err error) {
	expires = now.Add(ttl)
	jti = NewJTI()
	claims := &Claims{
		Stage: stage,
		SID:   sid,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("签发 JWT 失败: %w", err)
	}
	return signed, jti, expires, nil
}

// Parse 校验签名、结构与有效期。签名无效、结构错误、缺少 exp 或过期均返回错误。
// WithValidMethods 显式白名单 HS256，alg=none 与其他算法一律拒绝。
func (m *Manager) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}
