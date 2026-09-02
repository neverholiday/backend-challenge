package domain

// MaxPasswordLength is the longest password the system accepts, in bytes.
//
// The limit is bcrypt's: it hashes at most 72 bytes and rejects anything
// longer outright. It lives here rather than in the bcrypt adapter because it
// is a rule callers must respect to use the port at all - a hasher that
// silently truncated instead would be the more dangerous choice.
const MaxPasswordLength = 72

// PasswordHasher hashes plaintext passwords and verifies them against a stored hash.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) error
	// CompareDummy does the same work as Compare against a placeholder hash
	// that nothing matches, and discards the result. Callers use it on the
	// path where no stored hash exists - an unknown email at login - so that
	// branch costs the same time as a real comparison. Without it, an
	// unknown email answers measurably faster than a known one and the
	// difference is enough to enumerate registered accounts.
	CompareDummy(password string)
}
