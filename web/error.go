package web

import "github.com/buildset/buildset/pkg/httpx"

// Error is what a dependency returns when the request, not the system, is at fault. Kind is one of
// the sentinels above so errors.Is keeps working, and Message is written for the visitor, so a
// handler can show it without inspecting or reformatting an error chain.
type Error struct {
	Kind    error
	Message string
}

func NewError(kind error, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Kind
}

// ErrorFromCode turns a wire code into the sentinel this package uses, keeping the message the
// service wrote for the visitor.
//
// It lives here because these sentinels are this package's, so both the in-process adapter and the
// one that speaks HTTP map onto them the same way. An unknown code becomes nil: a code this
// binary does not understand is a failure of the system, and the caller must keep treating it as
// one rather than inventing an answer.
func ErrorFromCode(code httpx.Code, message string) error {
	switch code {
	case httpx.CodeNotFound:
		return NewError(ErrNotFound, message)
	case httpx.CodeInvalidInput:
		return NewError(ErrInvalidInput, message)
	case httpx.CodeConflict:
		return NewError(ErrConflict, message)
	default:
		return nil
	}
}
