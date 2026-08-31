package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

type ListUsers struct {
	repo domain.UserRepository
}

func NewListUsers(repo domain.UserRepository) *ListUsers {
	return &ListUsers{repo: repo}
}

func (uc *ListUsers) Execute(ctx context.Context) ([]domain.User, error) {
	users, err := uc.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	return users, nil
}
