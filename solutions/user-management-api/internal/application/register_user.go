package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// RegisterUserInput holds the fields needed to register a new user.
type RegisterUserInput struct {
	Name     string
	Email    string
	Password string
}

// RegisterUser is the use case for registering a new user.
type RegisterUser struct {
	repo   domain.UserRepository
	hasher domain.PasswordHasher
}

// NewRegisterUser builds a RegisterUser use case.
func NewRegisterUser(repo domain.UserRepository, hasher domain.PasswordHasher) *RegisterUser {
	return &RegisterUser{
		repo:   repo,
		hasher: hasher,
	}
}

// Execute validates in, hashes the password, and creates the user. It returns
// a *domain.ValidationError for malformed input and
// domain.ErrEmailAlreadyExists if the email is already registered.
func (uc *RegisterUser) Execute(ctx context.Context, in RegisterUserInput) (*domain.User, error) {
	if err := validateRegisterInput(in); err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	hashedPassword, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return nil, err
	}

	user := domain.User{
		ID:           id.String(),
		Name:         in.Name,
		Email:        in.Email,
		PasswordHash: hashedPassword,
		CreatedAt:    time.Now().UTC(),
	}

	err = uc.repo.CreateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// validateRegisterInput applies the domain's field rules. It lives in the use
// case rather than in each adapter so the HTTP and gRPC entry points cannot
// drift apart on what a valid registration is.
func validateRegisterInput(in RegisterUserInput) error {
	if err := domain.ValidateName(in.Name); err != nil {
		return err
	}
	if err := domain.ValidateEmail(in.Email); err != nil {
		return err
	}
	return domain.ValidatePassword(in.Password)
}
