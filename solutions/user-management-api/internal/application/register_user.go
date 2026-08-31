package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

type RegisterUserInput struct {
	Name     string
	Email    string
	Password string
}

type RegisterUser struct {
	repo   domain.UserRepository
	hasher domain.PasswordHasher
}

func NewRegisterUser(repo domain.UserRepository, hasher domain.PasswordHasher) *RegisterUser {
	return &RegisterUser{
		repo:   repo,
		hasher: hasher,
	}
}

func (uc *RegisterUser) Execute(ctx context.Context, in RegisterUserInput) (*domain.User, error) {
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
