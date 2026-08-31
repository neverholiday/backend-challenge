package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestDeleteUser_Execute(t *testing.T) {
	t.Run("deletes an existing user", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.users["user-1"] = domain.User{ID: "user-1", Email: "jane@example.com"}
		uc := application.NewDeleteUser(repo)

		err := uc.Execute(context.Background(), application.DeleteUserInput{ID: "user-1"})
		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if _, ok := repo.users["user-1"]; ok {
			t.Error("Execute() user still present after delete")
		}
	})

	t.Run("returns ErrUserNotFound for unknown id", func(t *testing.T) {
		repo := newFakeUserRepository()
		uc := application.NewDeleteUser(repo)

		err := uc.Execute(context.Background(), application.DeleteUserInput{ID: "missing"})
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("Execute() error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})
}
