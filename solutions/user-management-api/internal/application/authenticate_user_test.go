package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestAuthenticateUser_Execute(t *testing.T) {
	seedUser := domain.User{
		ID:           "user-1",
		Name:         "Jane Doe",
		Email:        "jane@example.com",
		PasswordHash: "hashed:s3cret",
	}

	t.Run("returns a token on valid credentials", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.users[seedUser.ID] = seedUser
		hasher := &fakePasswordHasher{}
		tokenService := &fakeTokenService{}
		uc := application.NewAuthenticateUser(repo, hasher, tokenService)

		token, err := uc.Execute(context.Background(), application.AuthenticateUserInput{
			Email:    "jane@example.com",
			Password: "s3cret",
		})
		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if token != "token:user-1" {
			t.Errorf("Execute() got token = %q, want token:user-1", token)
		}
	})

	t.Run("masks unknown email as invalid credentials", func(t *testing.T) {
		repo := newFakeUserRepository()
		hasher := &fakePasswordHasher{}
		tokenService := &fakeTokenService{}
		uc := application.NewAuthenticateUser(repo, hasher, tokenService)

		_, err := uc.Execute(context.Background(), application.AuthenticateUserInput{
			Email:    "nobody@example.com",
			Password: "s3cret",
		})
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Execute() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
		if hasher.DummyCompares != 1 {
			t.Errorf("Execute() made %d dummy comparisons, want 1 - an unknown email must cost the same as a known one", hasher.DummyCompares)
		}
	})

	t.Run("masks wrong password as invalid credentials", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.users[seedUser.ID] = seedUser
		hasher := &fakePasswordHasher{}
		tokenService := &fakeTokenService{}
		uc := application.NewAuthenticateUser(repo, hasher, tokenService)

		_, err := uc.Execute(context.Background(), application.AuthenticateUserInput{
			Email:    "jane@example.com",
			Password: "wrong-password",
		})
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Execute() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("propagates repo error other than not-found", func(t *testing.T) {
		repo := newFakeUserRepository()
		wantErr := errors.New("db boom")
		repo.GetUserByEmailErr = wantErr
		hasher := &fakePasswordHasher{}
		tokenService := &fakeTokenService{}
		uc := application.NewAuthenticateUser(repo, hasher, tokenService)

		_, err := uc.Execute(context.Background(), application.AuthenticateUserInput{
			Email:    "jane@example.com",
			Password: "s3cret",
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Execute() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("propagates token generation error", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.users[seedUser.ID] = seedUser
		hasher := &fakePasswordHasher{}
		wantErr := errors.New("token boom")
		tokenService := &fakeTokenService{GenerateTokenErr: wantErr}
		uc := application.NewAuthenticateUser(repo, hasher, tokenService)

		_, err := uc.Execute(context.Background(), application.AuthenticateUserInput{
			Email:    "jane@example.com",
			Password: "s3cret",
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Execute() error = %v, want %v", err, wantErr)
		}
	})
}
