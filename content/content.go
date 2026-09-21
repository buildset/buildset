// Package content owns the blogging domain: posts, their bodies, and their publication state. It
// holds no users and renders no pages. An author is a reference it stores and never resolves.
package content

import (
	"errors"
	"fmt"
	"slices"
)

const (
	// ServiceName is the service segment of every reference this package owns.
	ServiceName = "content"
	// PostResourceType is the type segment of a post reference.
	PostResourceType = "post"
)

var (
	ErrPostNotFound       = errors.New("post not found")
	ErrInvalidPost        = errors.New("invalid post")
	ErrInvalidStatus      = errors.New("invalid post status")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrUnsupportedContent = errors.New("unsupported content type")
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

// transitions is the whole lifecycle. A post leaves the archive as a draft, so republishing is a
// deliberate act rather than a side effect of unarchiving.
var transitions = map[Status][]Status{
	StatusDraft:     {StatusPublished, StatusArchived},
	StatusPublished: {StatusArchived},
	StatusArchived:  {StatusDraft},
}

func ParseStatus(value string) (Status, error) {
	status := Status(value)

	if _, ok := transitions[status]; !ok {
		return "", fmt.Errorf("%w: %q", ErrInvalidStatus, value)
	}

	return status, nil
}

// canTransition reports whether a post may move between two states. Staying put is allowed, so a
// resubmitted form does not fail.
func canTransition(from, to Status) bool {
	if from == to {
		return true
	}

	return slices.Contains(transitions[from], to)
}

// ContentTypePlainText is the only body format supported today.
const ContentTypePlainText = "text/plain"

func validateContentType(contentType string) error {
	if contentType != ContentTypePlainText {
		return fmt.Errorf("%w: %q", ErrUnsupportedContent, contentType)
	}

	return nil
}
