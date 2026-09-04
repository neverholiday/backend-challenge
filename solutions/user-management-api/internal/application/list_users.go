package application

import (
	"context"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// ListUsers is the use case for fetching every user.
type ListUsers struct {
	repo domain.UserRepository
}

// NewListUsers builds a ListUsers use case.
func NewListUsers(repo domain.UserRepository) *ListUsers {
	return &ListUsers{repo: repo}
}

// Execute returns every user.
func (uc *ListUsers) Execute(ctx context.Context) ([]domain.User, error) {
	users, err := uc.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	return users, nil
}
