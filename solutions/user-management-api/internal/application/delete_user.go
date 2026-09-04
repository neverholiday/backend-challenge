package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// DeleteUserInput identifies the user to delete.
type DeleteUserInput struct {
	ID string
}

// DeleteUser is the use case for removing a user.
type DeleteUser struct {
	repo domain.UserRepository
}

// NewDeleteUser builds a DeleteUser use case.
func NewDeleteUser(repo domain.UserRepository) *DeleteUser {
	return &DeleteUser{repo: repo}
}

// Execute deletes the user with id in.ID, or returns domain.ErrUserNotFound.
func (uc *DeleteUser) Execute(ctx context.Context, in DeleteUserInput) error {
	return uc.repo.DeleteUser(ctx, in.ID)
}
