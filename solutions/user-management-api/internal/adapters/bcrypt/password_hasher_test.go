package bcrypt_test

import (
	"errors"
	"strings"
	"testing"

	xbcrypt "golang.org/x/crypto/bcrypt"

	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/bcrypt"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

func TestHasher_HashAndCompare(t *testing.T) {
	h := bcrypt.NewHasher()

	t.Run("round trips a matching password", func(t *testing.T) {
		hash, err := h.Hash("s3cret-password")
		if err != nil {
			t.Fatalf("Hash() error = %v, want nil", err)
		}
		if hash == "s3cret-password" {
			t.Error("Hash() returned plaintext, want a hashed value")
		}

		if err := h.Compare(hash, "s3cret-password"); err != nil {
			t.Errorf("Compare() error = %v, want nil", err)
		}
	})

	t.Run("rejects a wrong password with ErrInvalidCredentials", func(t *testing.T) {
		hash, err := h.Hash("s3cret-password")
		if err != nil {
			t.Fatalf("Hash() error = %v, want nil", err)
		}

		err = h.Compare(hash, "wrong-password")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("Compare() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})

	t.Run("rejects a malformed hash with ErrInvalidCredentials", func(t *testing.T) {
		err := h.Compare("not-a-bcrypt-hash", "anything")
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("Compare() error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})
}

func TestHasher_Hash_PasswordLengthLimit(t *testing.T) {
	h := bcrypt.NewHasher()

	t.Run("accepts a password at the limit", func(t *testing.T) {
		password := strings.Repeat("a", domain.MaxPasswordLength)

		hash, err := h.Hash(password)
		if err != nil {
			t.Fatalf("Hash() error = %v, want nil", err)
		}
		if err := h.Compare(hash, password); err != nil {
			t.Errorf("Compare() error = %v, want nil", err)
		}
	})

	t.Run("rejects a password over the limit with ErrPasswordTooLong", func(t *testing.T) {
		_, err := h.Hash(strings.Repeat("a", domain.MaxPasswordLength+1))
		if !errors.Is(err, domain.ErrPasswordTooLong) {
			t.Errorf("Hash() error = %v, want %v", err, domain.ErrPasswordTooLong)
		}
	})
}

// TestHasher_CompareDummy covers the placeholder used to equalize the timing of
// a login against an unknown email. The cost assertion is the point: if the
// hasher's cost were raised without updating the placeholder, the dummy path
// would become measurably cheaper than a real comparison and the timing signal
// would come back.
func TestHasher_CompareDummy(t *testing.T) {
	h := bcrypt.NewHasher()

	t.Run("costs the same as a real comparison", func(t *testing.T) {
		real, err := h.Hash("s3cret-password")
		if err != nil {
			t.Fatalf("Hash() error = %v, want nil", err)
		}
		realCost, err := xbcrypt.Cost([]byte(real))
		if err != nil {
			t.Fatalf("Cost() error = %v, want nil", err)
		}

		dummyCost, err := xbcrypt.Cost([]byte(bcrypt.PlaceholderHashForTest))
		if err != nil {
			t.Fatalf("Cost() on the placeholder error = %v, want nil - it must be a valid bcrypt hash", err)
		}
		if dummyCost != realCost {
			t.Errorf("placeholder cost = %d, want %d", dummyCost, realCost)
		}
	})

	t.Run("does not panic and matches nothing", func(t *testing.T) {
		h.CompareDummy("any-password")

		if err := h.Compare(bcrypt.PlaceholderHashForTest, "any-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("Compare(placeholder, ...) error = %v, want %v", err, domain.ErrInvalidCredentials)
		}
	})
}
