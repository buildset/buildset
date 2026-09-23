// Package ref parses and formats the resource references services use to point at each other:
//
//	urn:<service>:<type>:<resource-id>
//
// A reference is opaque to every service except the one named in its service segment.
package ref

import (
	"errors"
	"fmt"
	"strings"
)

const (
	prefix       = "urn"
	separator    = ":"
	segmentCount = 4

	maxServiceLength      = 32
	maxResourceTypeLength = 32
	maxIDLength           = 128
)

var ErrInvalid = errors.New("invalid resource reference")

type Ref struct {
	Service      string
	ResourceType string
	ID           string
}

func New(service, resourceType, id string) (Ref, error) {
	r := Ref{Service: service, ResourceType: resourceType, ID: id}

	if err := r.Validate(); err != nil {
		return Ref{}, err
	}

	return r, nil
}

// MustNew is for references built from values the caller already controls, such as a fresh UUID and
// a constant service name. It panics on anything else.
func MustNew(service, resourceType, id string) Ref {
	r, err := New(service, resourceType, id)
	if err != nil {
		panic(err)
	}

	return r
}

func Parse(s string) (Ref, error) {
	segments := strings.Split(s, separator)
	if len(segments) != segmentCount {
		return Ref{}, fmt.Errorf("%w: want %d colon-separated segments, got %d", ErrInvalid, segmentCount, len(segments))
	}

	// RFC 8141 makes the scheme case-insensitive, but every stored reference is produced here, so
	// requiring lowercase keeps string comparison and indexing exact.
	if segments[0] != prefix {
		return Ref{}, fmt.Errorf("%w: want %q prefix, got %q", ErrInvalid, prefix, segments[0])
	}

	return New(segments[1], segments[2], segments[3])
}

// Validate reports whether a raw string is a well-formed reference.
func Validate(s string) error {
	_, err := Parse(s)

	return err
}

func (r Ref) Validate() error {
	if err := validateName("service", r.Service, maxServiceLength); err != nil {
		return err
	}

	if err := validateName("type", r.ResourceType, maxResourceTypeLength); err != nil {
		return err
	}

	return validateID(r.ID)
}

func (r Ref) String() string {
	return prefix + separator + r.Service + separator + r.ResourceType + separator + r.ID
}

// validateName accepts the lowercase, hyphen-separated words used for service and type segments.
func validateName(segment, value string, maxLength int) error {
	if value == "" {
		return fmt.Errorf("%w: %s segment is empty", ErrInvalid, segment)
	}

	if len(value) > maxLength {
		return fmt.Errorf("%w: %s segment is longer than %d characters", ErrInvalid, segment, maxLength)
	}

	for i, c := range value {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		case c == '-' && i > 0:
		default:
			return fmt.Errorf("%w: %s segment contains %q at position %d", ErrInvalid, segment, c, i)
		}
	}

	return nil
}

// validateID accepts the RFC 3986 unreserved set, which is URL-safe, colon-free, and wide enough
// for UUIDs, ULIDs, and anything else an owning service picks.
func validateID(value string) error {
	if value == "" {
		return fmt.Errorf("%w: id segment is empty", ErrInvalid)
	}

	if len(value) > maxIDLength {
		return fmt.Errorf("%w: id segment is longer than %d characters", ErrInvalid, maxIDLength)
	}

	for i, c := range value {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '.' || c == '_' || c == '~':
		default:
			return fmt.Errorf("%w: id segment contains %q at position %d", ErrInvalid, c, i)
		}
	}

	return nil
}
