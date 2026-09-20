package hash

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MaxBcryptPasswordLength is bcrypt's input limit in bytes. Longer input is silently truncated by
// the algorithm, which would let a different password match, so it is rejected instead.
const MaxBcryptPasswordLength = 72

type Bcrypt struct {
	cost int
}

func NewBcrypt(cost int) (*Bcrypt, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, fmt.Errorf("bcrypt cost must be between %d and %d, got %d", bcrypt.MinCost, bcrypt.MaxCost, cost)
	}

	return &Bcrypt{cost: cost}, nil
}

func (b *Bcrypt) Name() string {
	return "bcrypt"
}

// Identifiers covers every bcrypt revision in the wild. All three verify identically.
func (b *Bcrypt) Identifiers() []string {
	return []string{"2a", "2b", "2y"}
}

func (b *Bcrypt) Hash(password string) (string, error) {
	if len(password) > MaxBcryptPasswordLength {
		return "", fmt.Errorf("%w: bcrypt accepts at most %d bytes", ErrPasswordTooLong, MaxBcryptPasswordLength)
	}

	encoded, err := bcrypt.GenerateFromPassword([]byte(password), b.cost)
	if err != nil {
		return "", fmt.Errorf("generate bcrypt hash: %w", err)
	}

	return string(encoded), nil
}

func (b *Bcrypt) Verify(encoded, password string) error {
	if len(password) > MaxBcryptPasswordLength {
		return ErrMismatch
	}

	err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
	switch {
	case err == nil:
		return nil
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
		return ErrMismatch
	default:
		return fmt.Errorf("compare bcrypt hash: %w", err)
	}
}
