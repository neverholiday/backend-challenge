package domain

import "errors"

var (
	// ErrUserNotFound is returned when no user matches the given lookup.
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailAlreadyExists is returned when a user's email violates the unique constraint.
	ErrEmailAlreadyExists = errors.New("email already exists")
	// ErrInvalidCredentials is returned when email/password authentication fails.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken is returned when a token is missing, malformed, expired,
	// or fails signature verification.
	ErrInvalidToken = errors.New("invalid token")
	// ErrPasswordTooLong is returned when a password exceeds MaxPasswordLength.
	ErrPasswordTooLong = errors.New("password too long")
)
