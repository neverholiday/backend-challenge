package application

import (
	"context"
	"errors"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// AuthenticateUserInput holds the credentials to authenticate with.
type AuthenticateUserInput struct {
	Email    string
	Password string
}

// AuthenticateUser is the use case for verifying credentials and issuing a token.
type AuthenticateUser struct {
	repo         domain.UserRepository
	hasher       domain.PasswordHasher
	tokenService domain.TokenService
}

// NewAuthenticateUser builds an AuthenticateUser use case.
func NewAuthenticateUser(
	repo domain.UserRepository,
	hasher domain.PasswordHasher,
	tokenService domain.TokenService,
) *AuthenticateUser {
	return &AuthenticateUser{
		repo:         repo,
		hasher:       hasher,
		tokenService: tokenService,
	}
}

// Execute verifies in's credentials and returns a signed token, or
// domain.ErrInvalidCredentials if the email or password is wrong. The two
// failures are indistinguishable to the caller in both response and timing.
func (uc *AuthenticateUser) Execute(
	ctx context.Context,
	in AuthenticateUserInput,
) (string, error) {
	user, err := uc.repo.GetUserByEmail(ctx, in.Email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// Spend the cost of a hash comparison anyway. Returning here
			// directly would answer an unknown email far faster than a known
			// one with a wrong password, and that gap is a reliable oracle
			// for which addresses are registered.
			uc.hasher.CompareDummy(in.Password)
			return "", domain.ErrInvalidCredentials
		}
		return "", err
	}

	err = uc.hasher.Compare(user.PasswordHash, in.Password)
	if err != nil {
		return "", domain.ErrInvalidCredentials
	}

	token, err := uc.tokenService.GenerateToken(ctx, *user)
	if err != nil {
		return "", err
	}

	return token, nil
}
