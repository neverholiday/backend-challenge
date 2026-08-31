package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

type GetUserInput struct {
	ID string
}

type GetUser struct {
	repo domain.UserRepository
}

func NewGetUser(repo domain.UserRepository) *GetUser {
	return &GetUser{repo: repo}
}

func (uc *GetUser) Execute(ctx context.Context, in GetUserInput) (*domain.User, error) {
	user, err := uc.repo.GetUserByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return user, nil
}
