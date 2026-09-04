package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

// wantValidationError asserts that err is a *domain.ValidationError attributed
// to field. Adapters branch on the concrete type to pick a client-error status,
// so the type matters as much as the fact that an error was returned.
func wantValidationError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want a validation error for field %q", field)
	}
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("got error %v of type %T, want *domain.ValidationError", err, err)
	}
	if validationErr.Field != field {
		t.Errorf("Field = %q, want %q", validationErr.Field, field)
	}
	if validationErr.Message == "" {
		t.Error("Message is empty, want a client-safe explanation")
	}
	if validationErr.Error() != validationErr.Message {
		t.Errorf("Error() = %q, want %q", validationErr.Error(), validationErr.Message)
	}
}

func TestValidateName(t *testing.T) {
	t.Run("accepts a non-empty name", func(t *testing.T) {
		if err := domain.ValidateName("Jane Doe"); err != nil {
			t.Errorf("ValidateName() error = %v, want nil", err)
		}
	})

	t.Run("rejects an empty name", func(t *testing.T) {
		wantValidationError(t, domain.ValidateName(""), "name")
	})
}

func TestValidateEmail(t *testing.T) {
	valid := []string{
		"jane@example.com",
		"jane.doe+tag@sub.example.co.uk",
		"Jane Doe <jane@example.com>",
	}
	for _, email := range valid {
		t.Run("accepts "+email, func(t *testing.T) {
			if err := domain.ValidateEmail(email); err != nil {
				t.Errorf("ValidateEmail(%q) error = %v, want nil", email, err)
			}
		})
	}

	invalid := map[string]string{
		"empty":            "",
		"no at sign":       "jane.example.com",
		"no domain":        "jane@",
		"no local part":    "@example.com",
		"trailing comment": "jane@example.com (comment",
		"two addresses":    "jane@example.com, john@example.com",
	}
	for name, email := range invalid {
		t.Run("rejects "+name, func(t *testing.T) {
			wantValidationError(t, domain.ValidateEmail(email), "email")
		})
	}
}

func TestValidatePassword(t *testing.T) {
	t.Run("accepts a password at the minimum length", func(t *testing.T) {
		password := strings.Repeat("a", domain.MinPasswordLength)
		if err := domain.ValidatePassword(password); err != nil {
			t.Errorf("ValidatePassword() error = %v, want nil", err)
		}
	})

	t.Run("accepts a password at the maximum length", func(t *testing.T) {
		password := strings.Repeat("a", domain.MaxPasswordLength)
		if err := domain.ValidatePassword(password); err != nil {
			t.Errorf("ValidatePassword() error = %v, want nil", err)
		}
	})

	t.Run("rejects a password one byte under the minimum", func(t *testing.T) {
		password := strings.Repeat("a", domain.MinPasswordLength-1)
		wantValidationError(t, domain.ValidatePassword(password), "password")
	})

	t.Run("rejects a password one byte over the maximum", func(t *testing.T) {
		password := strings.Repeat("a", domain.MaxPasswordLength+1)
		wantValidationError(t, domain.ValidatePassword(password), "password")
	})

	// The bounds are measured in bytes because bcrypt truncates at 72 bytes,
	// not 72 runes: a multi-byte password can be short by rune count and still
	// be too long for the hasher.
	t.Run("measures length in bytes, not runes", func(t *testing.T) {
		password := strings.Repeat("é", 2)
		if len(password) < domain.MinPasswordLength {
			wantValidationError(t, domain.ValidatePassword(password), "password")
		}

		overlong := strings.Repeat("é", domain.MaxPasswordLength/2+1)
		if len([]rune(overlong)) > domain.MaxPasswordLength {
			t.Fatalf("test setup: %d runes, want at most %d so the failure is byte-driven",
				len([]rune(overlong)), domain.MaxPasswordLength)
		}
		wantValidationError(t, domain.ValidatePassword(overlong), "password")
	})
}

func TestUserUpdateParam_Validate(t *testing.T) {
	name := "Jane Updated"
	email := "jane.updated@example.com"
	empty := ""
	malformed := "not-an-email"

	t.Run("accepts a name-only patch", func(t *testing.T) {
		param := domain.UserUpdateParam{Name: &name}
		if err := param.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("accepts an email-only patch", func(t *testing.T) {
		param := domain.UserUpdateParam{Email: &email}
		if err := param.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("accepts a patch of both fields", func(t *testing.T) {
		param := domain.UserUpdateParam{Name: &name, Email: &email}
		if err := param.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("rejects an empty patch", func(t *testing.T) {
		wantValidationError(t, domain.UserUpdateParam{}.Validate(), "")
	})

	t.Run("rejects a present but empty name", func(t *testing.T) {
		param := domain.UserUpdateParam{Name: &empty}
		wantValidationError(t, param.Validate(), "name")
	})

	t.Run("rejects a present but malformed email", func(t *testing.T) {
		param := domain.UserUpdateParam{Email: &malformed}
		wantValidationError(t, param.Validate(), "email")
	})
}
