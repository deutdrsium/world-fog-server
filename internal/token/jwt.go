package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID      string `json:"uid"`
	DisplayName string `json:"name"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	expiry time.Duration
}

func NewManager(secret string, expiryHrs int) *Manager {
	return &Manager{
		secret: []byte(secret),
		expiry: time.Duration(expiryHrs) * time.Hour,
	}
}

func (m *Manager) Issue(userID, displayName string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:      userID,
		DisplayName: displayName,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiry)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *Manager) Verify(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
