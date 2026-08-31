package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

type DeleteUserInput struct {
	ID string
}

type DeleteUser struct {
	repo domain.UserRepository
}

func NewDeleteUser(repo domain.UserRepository) *DeleteUser {
	return &DeleteUser{repo: repo}
}

func (uc *DeleteUser) Execute(ctx context.Context, in DeleteUserInput) error {
	return uc.repo.DeleteUser(ctx, in.ID)
}
