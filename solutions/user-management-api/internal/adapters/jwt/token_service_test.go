package jwt_test

import (
	"context"
	"errors"
	"testing"
	"time"

	golangjwt "github.com/golang-jwt/jwt/v5"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/jwt"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestTokenService_GenerateAndValidate(t *testing.T) {
	svc := jwt.NewTokenService("test-secret", time.Hour)
	user := domain.User{ID: "user-123"}

	t.Run("validates a token it generated", func(t *testing.T) {
		token, err := svc.GenerateToken(context.Background(), user)
		if err != nil {
			t.Fatalf("GenerateToken() error = %v, want nil", err)
		}

		claims, err := svc.ValidateToken(context.Background(), token)
		if err != nil {
			t.Fatalf("ValidateToken() error = %v, want nil", err)
		}
		if claims.UserID != user.ID {
			t.Errorf("ValidateToken() UserID = %q, want %q", claims.UserID, user.ID)
		}
		if claims.ExpiresAt.Before(time.Now()) {
			t.Errorf("ValidateToken() ExpiresAt = %v, want a future time", claims.ExpiresAt)
		}
	})

	t.Run("rejects a garbage token", func(t *testing.T) {
		_, err := svc.ValidateToken(context.Background(), "not-a-jwt")
		if !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("ValidateToken() error = %v, want %v", err, domain.ErrInvalidToken)
		}
	})

	t.Run("rejects a token signed with a different secret", func(t *testing.T) {
		other := jwt.NewTokenService("other-secret", time.Hour)
		token, err := other.GenerateToken(context.Background(), user)
		if err != nil {
			t.Fatalf("GenerateToken() error = %v, want nil", err)
		}

		_, err = svc.ValidateToken(context.Background(), token)
		if !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("ValidateToken() error = %v, want %v", err, domain.ErrInvalidToken)
		}
	})

	t.Run("rejects an expired token", func(t *testing.T) {
		expired := jwt.NewTokenService("test-secret", -time.Hour)
		token, err := expired.GenerateToken(context.Background(), user)
		if err != nil {
			t.Fatalf("GenerateToken() error = %v, want nil", err)
		}

		_, err = svc.ValidateToken(context.Background(), token)
		if !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("ValidateToken() error = %v, want %v", err, domain.ErrInvalidToken)
		}
	})

	t.Run("rejects a token signed with an unexpected algorithm", func(t *testing.T) {
		claims := golangjwt.RegisteredClaims{
			Subject:   user.ID,
			ExpiresAt: golangjwt.NewNumericDate(time.Now().Add(time.Hour)),
		}
		token := golangjwt.NewWithClaims(golangjwt.SigningMethodNone, claims)
		signed, err := token.SignedString(golangjwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("SignedString() error = %v, want nil", err)
		}

		_, err = svc.ValidateToken(context.Background(), signed)
		if !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("ValidateToken() error = %v, want %v", err, domain.ErrInvalidToken)
		}
	})
}
