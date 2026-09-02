package application_test

import (
	"context"
	"errors"
	"strings"
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
			Password: "s3cret123",
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
		if got.PasswordHash != "hashed:s3cret123" {
			t.Errorf("Execute() got PasswordHash = %q, want hashed:s3cret123", got.PasswordHash)
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
			Password: "s3cret123",
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
			Password: "s3cret123",
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

func (h hashErrHasher) CompareDummy(string) {}

func (h hashErrHasher) Compare(string, string) error {
	return nil
}

func TestRegisterUser_Execute_Validation(t *testing.T) {
	valid := application.RegisterUserInput{
		Name:     "Jane Doe",
		Email:    "jane@example.com",
		Password: "s3cret123",
	}

	tests := []struct {
		name      string
		mutate    func(in *application.RegisterUserInput)
		wantField string
	}{
		{"empty name", func(in *application.RegisterUserInput) { in.Name = "" }, "name"},
		{"empty email", func(in *application.RegisterUserInput) { in.Email = "" }, "email"},
		{"malformed email", func(in *application.RegisterUserInput) { in.Email = "not-an-email" }, "email"},
		{"short password", func(in *application.RegisterUserInput) { in.Password = "short" }, "password"},
		{"overlong password", func(in *application.RegisterUserInput) {
			in.Password = strings.Repeat("a", domain.MaxPasswordLength+1)
		}, "password"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeUserRepository()
			uc := application.NewRegisterUser(repo, &fakePasswordHasher{})

			in := valid
			tt.mutate(&in)

			_, err := uc.Execute(context.Background(), in)

			var verr *domain.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Execute() error = %v, want *domain.ValidationError", err)
			}
			if verr.Field != tt.wantField {
				t.Errorf("Execute() error field = %q, want %q", verr.Field, tt.wantField)
			}
			if len(repo.users) != 0 {
				t.Errorf("Execute() stored %d users, want 0 - invalid input must not reach the repository", len(repo.users))
			}
		})
	}
}
