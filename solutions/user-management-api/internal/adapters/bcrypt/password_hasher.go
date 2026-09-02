// Package bcrypt implements domain.PasswordHasher using bcrypt.
package bcrypt

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// placeholderHash is a valid bcrypt hash of a random string that was never
// recorded anywhere, so no password can match it. CompareDummy verifies
// against it to spend the same time a real comparison would.
//
// It is a constant rather than generated at startup so that construction stays
// infallible and costs nothing; its cost factor is asserted against the
// hasher's own in the tests, so the two cannot drift apart.
const placeholderHash = "$2a$10$H7YVame4qlD9m/GcqrvCfu7kIw7BpbBZgIpzFTYPKaiEcnOjlHfXm"

// Hasher is a bcrypt-backed implementation of domain.PasswordHasher.
type Hasher struct {
	cost int
}

var _ domain.PasswordHasher = (*Hasher)(nil)

// NewHasher builds a Hasher using bcrypt.DefaultCost.
func NewHasher() *Hasher {
	return &Hasher{cost: bcrypt.DefaultCost}
}

// Hash returns the bcrypt hash of password. A password over
// domain.MaxPasswordLength bytes yields domain.ErrPasswordTooLong, so callers
// see a client error rather than an opaque failure from the bcrypt package.
func (h *Hasher) Hash(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return "", domain.ErrPasswordTooLong
	}
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// CompareDummy verifies password against a placeholder hash and throws the
// result away, so a caller with no stored hash to check still pays the cost of
// a comparison.
func (h *Hasher) CompareDummy(password string) {
	_ = bcrypt.CompareHashAndPassword([]byte(placeholderHash), []byte(password))
}

// Compare reports whether password matches hash. It returns
// domain.ErrInvalidCredentials on any mismatch so callers can branch on the
// sentinel without depending on the bcrypt package.
func (h *Hasher) Compare(hash string, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return domain.ErrInvalidCredentials
	}
	return nil
}
