package application_test

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// fakeUserRepository is an in-memory domain.UserRepository for tests.
// CreateUserErr / GetUserByIDErr / GetUserByEmailErr / UpdateUserErr / DeleteUserErr
// let a test force a specific error without touching the in-memory map.
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

func (f *fakeUserRepository) UpdateUser(
	_ context.Context,
	id string,
	param domain.UserUpdateParam,
) (*domain.User, error) {
	if f.UpdateUserErr != nil {
		return nil, f.UpdateUserErr
	}
	user, ok := f.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	if param.Name != nil {
		user.Name = *param.Name
	}
	if param.Email != nil {
		user.Email = *param.Email
	}
	f.users[id] = user
	return &user, nil
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

// fakePasswordHasher is a domain.PasswordHasher stub. Hash prefixes the
// input so tests can assert on it without a real bcrypt round trip.
// CompareErr lets a test force Compare to fail (wrong password path).
type fakePasswordHasher struct {
	CompareErr error
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
	if f.CompareErr != nil {
		return f.CompareErr
	}
	if hash != "hashed:"+password {
		return domain.ErrInvalidCredentials
	}
	return nil
}

// fakeTokenService is a domain.TokenService stub.
type fakeTokenService struct {
	GenerateTokenErr error
}

func (f *fakeTokenService) GenerateToken(_ context.Context, user domain.User) (string, error) {
	if f.GenerateTokenErr != nil {
		return "", f.GenerateTokenErr
	}
	return "token:" + user.ID, nil
}

func (f *fakeTokenService) ValidateToken(_ context.Context, token string) (domain.Claims, error) {
	return domain.Claims{}, nil
}
