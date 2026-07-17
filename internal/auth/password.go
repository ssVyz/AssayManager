// Package auth provides password hashing, in-memory sessions, and CSRF tokens
// for the MVP. Everything here is intentionally simple and process-local.
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of the given plaintext password.
func HashPassword(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plaintext matches the stored bcrypt hash.
func CheckPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
