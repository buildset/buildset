// Package hash turns passwords into storable strings and checks them again. Stored hashes carry
// their algorithm in a leading "$name$" segment, so a Registry can still verify a hash written by a
// superseded algorithm. That makes replacing bcrypt a configuration change, not a password reset.
package hash

import (
	"errors"
	"fmt"
	"strings"
)

// MinPasswordLength is the shortest password accepted, in bytes.
//
// TODO: length alone is weak. NIST SP 800-63B section 5.1.1.2 asks for a breached-password
// blocklist, which needs a data source this project does not have yet.
const MinPasswordLength = 8

var (
	ErrMismatch         = errors.New("password does not match hash")
	ErrUnknownAlgorithm = errors.New("unknown password hash algorithm")
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d bytes", MinPasswordLength)
	ErrPasswordTooLong  = errors.New("password is too long for the hash algorithm")

	errDuplicateIdentifier = errors.New("hash identifier is claimed twice")
)

// Algorithm is one password hashing scheme.
type Algorithm interface {
	// Name identifies the scheme. A stored hash written by a different name is rehashed on login.
	Name() string
	// Identifiers lists the "$name$" segments this scheme recognizes in a stored hash.
	Identifiers() []string
	Hash(password string) (string, error)
	Verify(encoded, password string) error
}

// Registry hashes with one preferred algorithm and verifies against every registered one.
type Registry struct {
	preferred  Algorithm
	algorithms map[string]Algorithm
}

func NewRegistry(preferred Algorithm, others ...Algorithm) (*Registry, error) {
	registry := &Registry{preferred: preferred, algorithms: make(map[string]Algorithm)}

	for _, algorithm := range append([]Algorithm{preferred}, others...) {
		for _, identifier := range algorithm.Identifiers() {
			if existing, ok := registry.algorithms[identifier]; ok {
				return nil, fmt.Errorf(
					"%w: %q claimed by both %s and %s",
					errDuplicateIdentifier,
					identifier,
					existing.Name(),
					algorithm.Name(),
				)
			}

			registry.algorithms[identifier] = algorithm
		}
	}

	return registry, nil
}

func (r *Registry) Hash(password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}

	return r.preferred.Hash(password)
}

func (r *Registry) Verify(encoded, password string) error {
	algorithm, err := r.algorithmFor(encoded)
	if err != nil {
		return err
	}

	return algorithm.Verify(encoded, password)
}

// NeedsRehash reports whether a stored hash was written by something other than the preferred
// algorithm. Callers rehash on a successful login, when the plaintext is in hand.
//
// TODO: also rehash when the preferred algorithm's cost parameter has been raised.
func (r *Registry) NeedsRehash(encoded string) bool {
	algorithm, err := r.algorithmFor(encoded)
	if err != nil {
		return true
	}

	return algorithm.Name() != r.preferred.Name()
}

func (r *Registry) algorithmFor(encoded string) (Algorithm, error) {
	identifier, err := identify(encoded)
	if err != nil {
		return nil, err
	}

	algorithm, ok := r.algorithms[identifier]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAlgorithm, identifier)
	}

	return algorithm, nil
}

// identify reads the algorithm segment of a "$name$rest" encoded hash.
func identify(encoded string) (string, error) {
	rest, ok := strings.CutPrefix(encoded, "$")
	if !ok {
		return "", fmt.Errorf("%w: hash does not start with $", ErrUnknownAlgorithm)
	}

	identifier, _, ok := strings.Cut(rest, "$")
	if !ok || identifier == "" {
		return "", fmt.Errorf("%w: hash has no algorithm segment", ErrUnknownAlgorithm)
	}

	return identifier, nil
}
