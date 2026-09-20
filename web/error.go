package web

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
