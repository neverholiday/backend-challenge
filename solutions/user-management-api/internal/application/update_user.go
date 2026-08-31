package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

type UpdateUserInput struct {
	ID    string
	Param domain.UserUpdateParam
}

type UpdateUser struct {
	repo domain.UserRepository
}

func NewUpdateUser(repo domain.UserRepository) *UpdateUser {
	return &UpdateUser{repo: repo}
}

func (uc *UpdateUser) Execute(ctx context.Context, in UpdateUserInput) error {
	return uc.repo.UpdateUser(ctx, in.ID, in.Param)
}
