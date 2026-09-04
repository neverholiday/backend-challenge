package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// UpdateUserInput identifies the user to update and the fields to change.
type UpdateUserInput struct {
	ID    string
	Param domain.UserUpdateParam
}

// UpdateUser is the use case for patching a user's name and/or email.
type UpdateUser struct {
	repo domain.UserRepository
}

// NewUpdateUser builds an UpdateUser use case.
func NewUpdateUser(repo domain.UserRepository) *UpdateUser {
	return &UpdateUser{repo: repo}
}

// Execute applies in.Param to the user with id in.ID and returns the updated
// user. It returns a *domain.ValidationError for an empty or malformed patch,
// or domain.ErrUserNotFound if no such user exists.
func (uc *UpdateUser) Execute(ctx context.Context, in UpdateUserInput) (*domain.User, error) {
	if err := in.Param.Validate(); err != nil {
		return nil, err
	}
	return uc.repo.UpdateUser(ctx, in.ID, in.Param)
}
