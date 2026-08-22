package domain

import (
	"context"
)

// UserRepository defines persistence operations for User.
type UserRepository interface {
	CreateUser(ctx context.Context, user User) error
	CountUsers(ctx context.Context) (uint, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	ListUsers(ctx context.Context) ([]User, error)
	UpdateUser(ctx context.Context, id string, param UserUpdateParam) error
	DeleteUser(ctx context.Context, id string) error
}
