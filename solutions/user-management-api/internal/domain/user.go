package domain

import "time"

// User represents a registered account in the system.
type User struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// UserUpdateParam holds the optional fields that can be patched on a User.
// A nil field is left unchanged.
type UserUpdateParam struct {
	Name  *string
	Email *string
}
