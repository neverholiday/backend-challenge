package domain

import "net/mail"

// MinPasswordLength is the shortest password accepted when registering.
//
// It is a policy of the domain rather than of any one adapter: the HTTP and
// gRPC entry points must not be able to disagree about what a valid password
// is, or an account created over one would be unusable over the other.
const MinPasswordLength = 8

// ValidationError reports input that violates a domain rule. Adapters map it
// to their transport's client-error status - HTTP 400, gRPC InvalidArgument -
// rather than treating it as an internal failure.
type ValidationError struct {
	// Field is the offending input field, for callers that want to attribute
	// the failure without parsing Message.
	Field string
	// Message is the human-readable explanation, safe to return to a client.
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func newValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// ValidateName reports whether name is acceptable for a user.
func ValidateName(name string) error {
	if name == "" {
		return newValidationError("name", "name is required")
	}
	return nil
}

// ValidateEmail reports whether email is present and well formed.
func ValidateEmail(email string) error {
	if email == "" {
		return newValidationError("email", "email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return newValidationError("email", "email is not a valid address")
	}
	return nil
}

// ValidatePassword reports whether password satisfies the length policy. The
// bounds are in bytes, matching how the hasher measures them.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return newValidationError("password", "password must be at least 8 characters")
	}
	if len(password) > MaxPasswordLength {
		return newValidationError("password", "password must not exceed 72 bytes")
	}
	return nil
}

// Validate reports whether the patch is applicable: at least one field must be
// present, and every present field must itself be valid.
func (p UserUpdateParam) Validate() error {
	if p.Name == nil && p.Email == nil {
		return newValidationError("", "at least one of name or email is required")
	}
	if p.Name != nil {
		if err := ValidateName(*p.Name); err != nil {
			return err
		}
	}
	if p.Email != nil {
		if err := ValidateEmail(*p.Email); err != nil {
			return err
		}
	}
	return nil
}
