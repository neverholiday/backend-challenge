package grpc_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcadapter "github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/grpc"
	userv1 "github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/grpc/userv1"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// fakeUserRepository is a minimal in-memory domain.UserRepository for tests.
type fakeUserRepository struct {
	users map[string]domain.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{users: make(map[string]domain.User)}
}

func (f *fakeUserRepository) CreateUser(_ context.Context, user domain.User) error {
	for _, existing := range f.users {
		if existing.Email == user.Email {
			return domain.ErrEmailAlreadyExists
		}
	}
	f.users[user.ID] = user
	return nil
}

func (f *fakeUserRepository) CountUsers(_ context.Context) (uint, error) {
	return uint(len(f.users)), nil
}

func (f *fakeUserRepository) GetUserByID(_ context.Context, id string) (*domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return &user, nil
}

func (f *fakeUserRepository) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, user := range f.users {
		if user.Email == email {
			u := user
			return &u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUserRepository) ListUsers(_ context.Context) ([]domain.User, error) {
	users := make([]domain.User, 0, len(f.users))
	for _, user := range f.users {
		users = append(users, user)
	}
	return users, nil
}

func (f *fakeUserRepository) UpdateUser(context.Context, string, domain.UserUpdateParam) (*domain.User, error) {
	panic("not used")
}

func (f *fakeUserRepository) DeleteUser(context.Context, string) error {
	panic("not used")
}

type fakePasswordHasher struct {
	HashErr error
	// DummyCompares counts CompareDummy calls; unused here, but the field
	// keeps the fake identical to the other packages' stubs.
	DummyCompares int
}

func (f *fakePasswordHasher) Hash(password string) (string, error) {
	if f.HashErr != nil {
		return "", f.HashErr
	}
	return "hashed:" + password, nil
}

func (f *fakePasswordHasher) CompareDummy(string) {
	f.DummyCompares++
}

func (f *fakePasswordHasher) Compare(hash string, password string) error {
	if hash != "hashed:"+password {
		return domain.ErrInvalidCredentials
	}
	return nil
}

func newTestServer() (*grpcadapter.Server, *fakeUserRepository) {
	return newTestServerWithHasher(&fakePasswordHasher{})
}

func newTestServerWithHasher(hasher *fakePasswordHasher) (*grpcadapter.Server, *fakeUserRepository) {
	repo := newFakeUserRepository()
	registerUser := application.NewRegisterUser(repo, hasher)
	getUser := application.NewGetUser(repo)
	return grpcadapter.NewServer(registerUser, getUser), repo
}

func TestServer_CreateUser(t *testing.T) {
	t.Run("creates a user", func(t *testing.T) {
		s, _ := newTestServer()

		resp, err := s.CreateUser(t.Context(), &userv1.CreateUserRequest{
			Name: "Jane Doe", Email: "jane@example.com", Password: "s3cret123",
		})
		if err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}
		if resp.GetId() == "" {
			t.Error("CreateUser() got empty id, want generated id")
		}
		if resp.GetEmail() != "jane@example.com" {
			t.Errorf("CreateUser() got Email = %q, want jane@example.com", resp.GetEmail())
		}
	})

	t.Run("rejects a missing field with InvalidArgument", func(t *testing.T) {
		s, _ := newTestServer()

		_, err := s.CreateUser(t.Context(), &userv1.CreateUserRequest{Name: "Jane Doe"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("CreateUser() code = %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("maps a duplicate email to AlreadyExists", func(t *testing.T) {
		s, _ := newTestServer()

		req := &userv1.CreateUserRequest{Name: "Jane Doe", Email: "dup@example.com", Password: "s3cret123"}
		if _, err := s.CreateUser(t.Context(), req); err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}

		_, err := s.CreateUser(t.Context(), req)
		if status.Code(err) != codes.AlreadyExists {
			t.Errorf("CreateUser() code = %v, want AlreadyExists", status.Code(err))
		}
	})
}

func TestServer_GetUser(t *testing.T) {
	t.Run("returns an existing user", func(t *testing.T) {
		s, repo := newTestServer()
		if err := repo.CreateUser(t.Context(), domain.User{ID: "user-1", Name: "Jane", Email: "jane@example.com"}); err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}

		resp, err := s.GetUser(t.Context(), &userv1.GetUserRequest{Id: "user-1"})
		if err != nil {
			t.Fatalf("GetUser() error = %v, want nil", err)
		}
		if resp.GetId() != "user-1" {
			t.Errorf("GetUser() got Id = %q, want user-1", resp.GetId())
		}
	})

	t.Run("maps a missing user to NotFound", func(t *testing.T) {
		s, _ := newTestServer()

		_, err := s.GetUser(t.Context(), &userv1.GetUserRequest{Id: "does-not-exist"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("GetUser() code = %v, want NotFound", status.Code(err))
		}
	})

	t.Run("rejects an empty id with InvalidArgument", func(t *testing.T) {
		s, _ := newTestServer()

		_, err := s.GetUser(t.Context(), &userv1.GetUserRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("GetUser() code = %v, want InvalidArgument", status.Code(err))
		}
	})
}

func TestServer_CreateUser_PasswordTooLong(t *testing.T) {
	s, _ := newTestServerWithHasher(&fakePasswordHasher{HashErr: domain.ErrPasswordTooLong})

	_, err := s.CreateUser(t.Context(), &userv1.CreateUserRequest{
		Name:     "Jane",
		Email:    "jane@example.com",
		Password: strings.Repeat("a", domain.MaxPasswordLength+1),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("CreateUser() code = %v, want %v", status.Code(err), codes.InvalidArgument)
	}
}

// TestServer_CreateUser_ValidationMatchesHTTP pins the rules the gRPC adapter
// enforces. They are the domain's, applied by the use case, so this adapter
// cannot drift into accepting registrations the HTTP adapter would reject.
func TestServer_CreateUser_ValidationMatchesHTTP(t *testing.T) {
	tests := []struct {
		name string
		req  *userv1.CreateUserRequest
	}{
		{"empty name", &userv1.CreateUserRequest{Email: "jane@example.com", Password: "s3cret123"}},
		{"empty email", &userv1.CreateUserRequest{Name: "Jane", Password: "s3cret123"}},
		{"malformed email", &userv1.CreateUserRequest{Name: "Jane", Email: "not-an-email", Password: "s3cret123"}},
		{"empty password", &userv1.CreateUserRequest{Name: "Jane", Email: "jane@example.com"}},
		{"short password", &userv1.CreateUserRequest{Name: "Jane", Email: "jane@example.com", Password: "short"}},
		{"overlong password", &userv1.CreateUserRequest{
			Name:     "Jane",
			Email:    "jane@example.com",
			Password: strings.Repeat("a", domain.MaxPasswordLength+1),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, repo := newTestServer()

			_, err := s.CreateUser(t.Context(), tt.req)

			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("CreateUser() code = %v, want %v", status.Code(err), codes.InvalidArgument)
			}
			if len(repo.users) != 0 {
				t.Errorf("CreateUser() stored %d users, want 0", len(repo.users))
			}
		})
	}
}
