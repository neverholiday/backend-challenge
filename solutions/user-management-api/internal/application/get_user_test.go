package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestGetUser_Execute(t *testing.T) {
	t.Run("returns the user by id", func(t *testing.T) {
		repo := newFakeUserRepository()
		want := domain.User{ID: "user-1", Name: "Jane Doe", Email: "jane@example.com"}
		repo.users[want.ID] = want
		uc := application.NewGetUser(repo)

		got, err := uc.Execute(context.Background(), application.GetUserInput{ID: "user-1"})
		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if got.Email != want.Email {
			t.Errorf("Execute() got Email = %q, want %q", got.Email, want.Email)
		}
	})

	t.Run("returns ErrUserNotFound for unknown id", func(t *testing.T) {
		repo := newFakeUserRepository()
		uc := application.NewGetUser(repo)

		_, err := uc.Execute(context.Background(), application.GetUserInput{ID: "missing"})
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("Execute() error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})
}
