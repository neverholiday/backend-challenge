package domain

// PasswordHasher hashes plaintext passwords and verifies them against a stored hash.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) error
}
