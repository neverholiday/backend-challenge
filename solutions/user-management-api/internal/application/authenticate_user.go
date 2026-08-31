package application

import (
	"context"
	"errors"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

type AuthenticateUserInput struct {
	Email    string
	Password string
}

type AuthenticateUser struct {
	repo         domain.UserRepository
	hasher       domain.PasswordHasher
	tokenService domain.TokenService
}

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

func (uc *AuthenticateUser) Execute(
	ctx context.Context,
	in AuthenticateUserInput,
) (string, error) {
	user, err := uc.repo.GetUserByEmail(ctx, in.Email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
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
