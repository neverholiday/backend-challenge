package application_test

import (
	"context"
	"testing"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestListUsers_Execute(t *testing.T) {
	t.Run("returns all users", func(t *testing.T) {
		repo := newFakeUserRepository()
		repo.users["user-1"] = domain.User{ID: "user-1", Email: "jane@example.com"}
		repo.users["user-2"] = domain.User{ID: "user-2", Email: "john@example.com"}
		uc := application.NewListUsers(repo)

		got, err := uc.Execute(context.Background())
		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if len(got) != 2 {
			t.Fatalf("Execute() got %d users, want 2", len(got))
		}
	})

	t.Run("returns empty slice when no users exist", func(t *testing.T) {
		repo := newFakeUserRepository()
		uc := application.NewListUsers(repo)

		got, err := uc.Execute(context.Background())
		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Fatalf("Execute() got %d users, want 0", len(got))
		}
	})
}
