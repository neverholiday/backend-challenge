//go:build integration

package mongodb_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/mongodb"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// mongoURI is set once in TestMain and shared by every test in this file so
// only one container spins up for the whole run.
var mongoURI string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcmongodb.Run(ctx, "mongo:7")
	if err != nil {
		panic(err)
	}
	defer func() { _ = container.Terminate(ctx) }()

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		panic(err)
	}
	mongoURI = uri

	os.Exit(m.Run())
}

// newTestRepository connects to the shared container and returns a repository
// backed by a database unique to the calling (sub)test, so tests never see
// each other's documents despite sharing one container.
func newTestRepository(t *testing.T) *mongodb.UserRepository {
	t.Helper()
	ctx := context.Background()

	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("mongo.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	dbName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	repo := mongodb.NewUserRepository(client.Database(dbName))
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes() error = %v", err)
	}
	return repo
}

func TestUserRepository_CreateUser(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	user := domain.User{
		ID:           "user-1",
		Name:         "Jane Doe",
		Email:        "jane@example.com",
		PasswordHash: "hashed",
		CreatedAt:    time.Now().UTC(),
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	got, err := repo.GetUserByID(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetUserByID() error = %v, want nil", err)
	}
	if got.Email != user.Email {
		t.Errorf("GetUserByID() got Email = %q, want %q", got.Email, user.Email)
	}

	t.Run("duplicate email is rejected by the unique index", func(t *testing.T) {
		dup := domain.User{ID: "user-2", Email: "jane@example.com"}
		err := repo.CreateUser(ctx, dup)
		if !errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Fatalf("CreateUser() error = %v, want %v", err, domain.ErrEmailAlreadyExists)
		}
	})
}

func TestUserRepository_GetUserByID_NotFound(t *testing.T) {
	repo := newTestRepository(t)

	_, err := repo.GetUserByID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("GetUserByID() error = %v, want %v", err, domain.ErrUserNotFound)
	}
}

func TestUserRepository_GetUserByEmail(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	if err := repo.CreateUser(ctx, domain.User{ID: "user-1", Email: "jane@example.com"}); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	t.Run("found", func(t *testing.T) {
		got, err := repo.GetUserByEmail(ctx, "jane@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v, want nil", err)
		}
		if got.ID != "user-1" {
			t.Errorf("GetUserByEmail() got ID = %q, want user-1", got.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetUserByEmail(ctx, "missing@example.com")
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetUserByEmail() error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})
}

func TestUserRepository_ListUsers(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	for _, u := range []domain.User{
		{ID: "user-1", Email: "jane@example.com"},
		{ID: "user-2", Email: "john@example.com"},
	} {
		if err := repo.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}
	}

	got, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListUsers() got %d users, want 2", len(got))
	}
}

func TestUserRepository_CountUsers(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	for _, u := range []domain.User{
		{ID: "user-1", Email: "jane@example.com"},
		{ID: "user-2", Email: "john@example.com"},
		{ID: "user-3", Email: "jack@example.com"},
	} {
		if err := repo.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}
	}

	got, err := repo.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers() error = %v, want nil", err)
	}
	if got != 3 {
		t.Fatalf("CountUsers() got = %d, want 3", got)
	}
}

func TestUserRepository_UpdateUser(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	if err := repo.CreateUser(ctx, domain.User{ID: "user-1", Name: "Jane Doe", Email: "jane@example.com"}); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	if err := repo.CreateUser(ctx, domain.User{ID: "user-2", Email: "john@example.com"}); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	t.Run("updates name and email", func(t *testing.T) {
		newName := "Jane Smith"
		newEmail := "jane.smith@example.com"
		err := repo.UpdateUser(ctx, "user-1", domain.UserUpdateParam{Name: &newName, Email: &newEmail})
		if err != nil {
			t.Fatalf("UpdateUser() error = %v, want nil", err)
		}

		got, err := repo.GetUserByID(ctx, "user-1")
		if err != nil {
			t.Fatalf("GetUserByID() error = %v, want nil", err)
		}
		if got.Name != newName || got.Email != newEmail {
			t.Errorf("got Name/Email = %q/%q, want %q/%q", got.Name, got.Email, newName, newEmail)
		}
	})

	t.Run("not found", func(t *testing.T) {
		newName := "Nobody"
		err := repo.UpdateUser(ctx, "missing", domain.UserUpdateParam{Name: &newName})
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("UpdateUser() error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("duplicate email is rejected by the unique index", func(t *testing.T) {
		taken := "john@example.com"
		err := repo.UpdateUser(ctx, "user-1", domain.UserUpdateParam{Email: &taken})
		if !errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Fatalf("UpdateUser() error = %v, want %v", err, domain.ErrEmailAlreadyExists)
		}
	})
}

func TestUserRepository_DeleteUser(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	if err := repo.CreateUser(ctx, domain.User{ID: "user-1", Email: "jane@example.com"}); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	t.Run("deletes an existing user", func(t *testing.T) {
		if err := repo.DeleteUser(ctx, "user-1"); err != nil {
			t.Fatalf("DeleteUser() error = %v, want nil", err)
		}
		_, err := repo.GetUserByID(ctx, "user-1")
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetUserByID() after delete error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.DeleteUser(ctx, "missing")
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("DeleteUser() error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})
}
