package http_test

import (
	"context"
	"time"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// fakeUserRepository is an in-memory domain.UserRepository for tests.
type fakeUserRepository struct {
	users map[string]domain.User

	CreateUserErr     error
	GetUserByIDErr    error
	GetUserByEmailErr error
	UpdateUserErr     error
	DeleteUserErr     error
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{users: make(map[string]domain.User)}
}

func (f *fakeUserRepository) CreateUser(_ context.Context, user domain.User) error {
	if f.CreateUserErr != nil {
		return f.CreateUserErr
	}
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
	if f.GetUserByIDErr != nil {
		return nil, f.GetUserByIDErr
	}
	user, ok := f.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return &user, nil
}

func (f *fakeUserRepository) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	if f.GetUserByEmailErr != nil {
		return nil, f.GetUserByEmailErr
	}
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

func (f *fakeUserRepository) UpdateUser(_ context.Context, id string, param domain.UserUpdateParam) error {
	if f.UpdateUserErr != nil {
		return f.UpdateUserErr
	}
	user, ok := f.users[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	if param.Name != nil {
		user.Name = *param.Name
	}
	if param.Email != nil {
		user.Email = *param.Email
	}
	f.users[id] = user
	return nil
}

func (f *fakeUserRepository) DeleteUser(_ context.Context, id string) error {
	if f.DeleteUserErr != nil {
		return f.DeleteUserErr
	}
	if _, ok := f.users[id]; !ok {
		return domain.ErrUserNotFound
	}
	delete(f.users, id)
	return nil
}

// fakePasswordHasher is a domain.PasswordHasher stub.
type fakePasswordHasher struct {
	// DummyCompares counts CompareDummy calls, so tests can assert the
	// unknown-email path still pays for a comparison.
	DummyCompares int
}

func (f *fakePasswordHasher) Hash(password string) (string, error) {
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

// fakeTokenService is a domain.TokenService stub. Tokens are the literal
// user ID, and any other value fails validation.
type fakeTokenService struct{}

func (f *fakeTokenService) GenerateToken(_ context.Context, user domain.User) (string, error) {
	return user.ID, nil
}

func (f *fakeTokenService) ValidateToken(_ context.Context, token string) (domain.Claims, error) {
	if token == "" || token == "invalid" {
		return domain.Claims{}, domain.ErrInvalidToken
	}
	return domain.Claims{UserID: token, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
