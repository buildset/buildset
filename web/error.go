package web

import "github.com/buildset/buildset/pkg/httpx"

// Error is what a dependency returns when the request, not the system, is at fault. Kind is one of
// the sentinels above so errors.Is keeps working; Message is written for the visitor.
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
// service wrote for the visitor. It lives here so both adapters map onto them the same way. An
// unknown code becomes nil, and the caller must keep treating it as a failure of the system.
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
