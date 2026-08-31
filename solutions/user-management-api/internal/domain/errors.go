package domain

import "errors"

var (
	// ErrUserNotFound is returned when no user matches the given lookup.
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailAlreadyExists is returned when a user's email violates the unique constraint.
	ErrEmailAlreadyExists = errors.New("email already exists")
	// ErrInvalidCredentials is returned when email/password authentication fails.
	ErrInvalidCredentials = errors.New("invalid credentials")
)
