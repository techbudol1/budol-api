package session

import (
	"time"

	"github.com/techbudol1/budol-api/internal/store"

	"github.com/golang-jwt/jwt/v5"
)

type Manager struct {
	secret []byte
	ttl    time.Duration
}

type Claims struct {
	WalletAddress string `json:"walletAddress"`
	jwt.RegisteredClaims
}

func NewManager(secret string, ttl time.Duration) Manager {
	return Manager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

func (m Manager) TTL() time.Duration {
	return m.ttl
}

func (m Manager) Issue(user store.User) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		WalletAddress: user.WalletAddress,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m Manager) Verify(tokenValue string) (Claims, error) {
	claims := Claims{}
	token, err := jwt.ParseWithClaims(tokenValue, &claims, func(token *jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return Claims{}, err
	}
	if !token.Valid {
		return Claims{}, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
