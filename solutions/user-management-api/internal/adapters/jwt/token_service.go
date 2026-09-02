// Package jwt implements domain.TokenService using HS256-signed JWTs.
package jwt

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// TokenService issues and validates HS256 JWTs whose subject is the user ID.
type TokenService struct {
	secret []byte
	ttl    time.Duration
}

var _ domain.TokenService = (*TokenService)(nil)

// NewTokenService builds a TokenService signing tokens with secret and
// setting them to expire after ttl.
func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl}
}

// GenerateToken issues a signed token for user, valid for the configured TTL.
func (s *TokenService) GenerateToken(_ context.Context, user domain.User) (string, error) {
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   user.ID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateToken parses and verifies raw, returning domain.ErrInvalidToken for
// any malformed, expired, or wrong-algorithm token rather than leaking the
// underlying jwt library's error type across the port boundary.
func (s *TokenService) ValidateToken(_ context.Context, raw string) (domain.Claims, error) {
	claims := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return domain.Claims{}, domain.ErrInvalidToken
	}
	if claims.Subject == "" || claims.ExpiresAt == nil {
		return domain.Claims{}, domain.ErrInvalidToken
	}

	return domain.Claims{
		UserID:    claims.Subject,
		ExpiresAt: claims.ExpiresAt.Time,
	}, nil
}
