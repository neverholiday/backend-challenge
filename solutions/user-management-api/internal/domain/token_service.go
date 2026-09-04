package domain

import (
	"context"
	"time"
)

// Claims holds the identity data extracted from a validated token.
type Claims struct {
	UserID    string
	ExpiresAt time.Time
}

// TokenService issues and validates authentication tokens.
type TokenService interface {
	GenerateToken(ctx context.Context, user User) (string, error)
	ValidateToken(ctx context.Context, token string) (Claims, error)
}
