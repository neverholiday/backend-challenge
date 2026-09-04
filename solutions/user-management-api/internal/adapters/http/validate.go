package http

import (
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// validationError represents a transport-level input error, mapped to HTTP 400.
// Field-level rules (name, email, password) are not enforced here - they live
// in the domain and are applied by the use cases, so the gRPC adapter enforces
// exactly the same ones. This type covers what only HTTP can see, such as a
// body that is not valid JSON.
type validationError struct {
	msg string
}

func newValidationError(msg string) *validationError {
	return &validationError{msg: msg}
}

func (e *validationError) Error() string {
	return e.msg
}

// validateLoginRequest checks the credentials are present and well formed
// before a lookup is attempted. Login is an HTTP-only endpoint, so unlike the
// registration rules there is no second adapter to keep in step.
func validateLoginRequest(req loginRequest) error {
	if err := domain.ValidateEmail(req.Email); err != nil {
		return err
	}
	if req.Password == "" {
		return newValidationError("password is required")
	}
	return nil
}
