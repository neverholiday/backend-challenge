package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestUpdateUser_Execute(t *testing.T) {
	t.Run("updates name and email", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.users["user-1"] = domain.User{ID: "user-1", Name: "Jane Doe", Email: "jane@example.com"}
		uc := application.NewUpdateUser(repo)

		newName := "Jane Smith"
		newEmail := "jane.smith@example.com"
		err := uc.Execute(context.Background(), application.UpdateUserInput{
			ID: "user-1",
			Param: domain.UserUpdateParam{
				Name:  &newName,
				Email: &newEmail,
			},
		})
		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}

		got := repo.users["user-1"]
		if got.Name != newName || got.Email != newEmail {
			t.Errorf("Execute() got Name/Email = %q/%q, want %q/%q", got.Name, got.Email, newName, newEmail)
		}
	})

	t.Run("returns ErrUserNotFound for unknown id", func(t *testing.T) {
		repo := newFakeUserRepository()
		uc := application.NewUpdateUser(repo)

		newName := "Jane Smith"
		err := uc.Execute(context.Background(), application.UpdateUserInput{
			ID:    "missing",
			Param: domain.UserUpdateParam{Name: &newName},
		})
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("Execute() error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("propagates duplicate email error from repo", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.users["user-1"] = domain.User{ID: "user-1", Email: "jane@example.com"}
		repo.UpdateUserErr = domain.ErrEmailAlreadyExists
		uc := application.NewUpdateUser(repo)

		newEmail := "taken@example.com"
		err := uc.Execute(context.Background(), application.UpdateUserInput{
			ID:    "user-1",
			Param: domain.UserUpdateParam{Email: &newEmail},
		})
		if !errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Fatalf("Execute() error = %v, want %v", err, domain.ErrEmailAlreadyExists)
		}
	})
}
