package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestRegisterUser_Execute(t *testing.T) {
	t.Run("creates a user with a hashed password and generated id", func(t *testing.T) {
		repo := newFakeUserRepository()
		hasher := &fakePasswordHasher{}
		uc := application.NewRegisterUser(repo, hasher)

		got, err := uc.Execute(context.Background(), application.RegisterUserInput{
			Name:     "Jane Doe",
			Email:    "jane@example.com",
			Password: "s3cret",
		})
		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if got == nil {
			t.Fatal("Execute() got nil user, want non-nil")
		}
		if got.ID == "" {
			t.Error("Execute() got empty ID, want generated id")
		}
		if got.Name != "Jane Doe" || got.Email != "jane@example.com" {
			t.Errorf("Execute() got Name/Email = %q/%q, want Jane Doe/jane@example.com", got.Name, got.Email)
		}
		if got.PasswordHash != "hashed:s3cret" {
			t.Errorf("Execute() got PasswordHash = %q, want hashed:s3cret", got.PasswordHash)
		}
		if got.CreatedAt.IsZero() {
			t.Error("Execute() got zero CreatedAt, want set")
		}

		stored, err := repo.GetUserByID(context.Background(), got.ID)
		if err != nil {
			t.Fatalf("repo.GetUserByID() error = %v, want nil", err)
		}
		if stored.Email != "jane@example.com" {
			t.Errorf("stored user Email = %q, want jane@example.com", stored.Email)
		}
	})

	t.Run("propagates hasher error", func(t *testing.T) {
		repo := newFakeUserRepository()
		wantErr := errors.New("hash boom")
		uc := application.NewRegisterUser(repo, hashErrHasher{err: wantErr})

		_, err := uc.Execute(context.Background(), application.RegisterUserInput{
			Name:     "Jane Doe",
			Email:    "jane@example.com",
			Password: "s3cret",
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Execute() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("propagates duplicate email error from repo", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.CreateUserErr = domain.ErrEmailAlreadyExists
		hasher := &fakePasswordHasher{}
		uc := application.NewRegisterUser(repo, hasher)

		_, err := uc.Execute(context.Background(), application.RegisterUserInput{
			Name:     "Jane Doe",
			Email:    "jane@example.com",
			Password: "s3cret",
		})
		if !errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Fatalf("Execute() error = %v, want %v", err, domain.ErrEmailAlreadyExists)
		}
	})
}

// hashErrHasher is a domain.PasswordHasher whose Hash always fails.
type hashErrHasher struct {
	err error
}

func (h hashErrHasher) Hash(string) (string, error) {
	return "", h.err
}

func (h hashErrHasher) Compare(string, string) error {
	return nil
}
