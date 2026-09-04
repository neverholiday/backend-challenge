package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// GetUserInput identifies the user to fetch.
type GetUserInput struct {
	ID string
}

// GetUser is the use case for fetching a single user by id.
type GetUser struct {
	repo domain.UserRepository
}

// NewGetUser builds a GetUser use case.
func NewGetUser(repo domain.UserRepository) *GetUser {
	return &GetUser{repo: repo}
}

// Execute returns the user with the given id, or domain.ErrUserNotFound.
func (uc *GetUser) Execute(ctx context.Context, in GetUserInput) (*domain.User, error) {
	user, err := uc.repo.GetUserByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return user, nil
}
