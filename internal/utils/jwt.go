package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	jwtSecret []byte
	jwtExpire time.Duration
	jwtInited bool
)

// InitJWTAuth 必须在生成/解析任何 token 之前调用（app 启动时），
// 否则 GenerateToken / ParseToken 会 panic，避免退化为空密钥签名。
func InitJWTAuth(secret string, expireMinutes int) {
	if secret == "" {
		panic("jwt secret is empty: set jwt.secret in config or $JWT_SECRET")
	}
	if expireMinutes <= 0 {
		expireMinutes = 24 * 60
	}
	jwtSecret = []byte(secret)
	jwtExpire = time.Duration(expireMinutes) * time.Minute
	jwtInited = true
}

func jwtReady() {
	if !jwtInited {
		panic("jwt auth not initialized: call utils.InitJWTAuth at startup")
	}
}

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateToken(userId string, username string) (string, error) {
	jwtReady()
	now := time.Now()
	claims := &Claims{
		UserID:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(jwtExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "ggb_server",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", errors.New("failed to sign token")
	}

	return tokenString, nil
}

func ParseToken(tokenString string) (*Claims, error) {
	jwtReady()
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("token is expired")
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, errors.New("token is malformed")
		}
		if errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, errors.New("token not active yet")
		}
		return nil, errors.New("failed to parse token")
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
