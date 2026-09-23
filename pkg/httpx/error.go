// Package httpx is the contract between services: a JSON request, a JSON response, and one shape
// for a failure the request rather than the system is at fault for. It imports nothing but the
// standard library and pkg/reqid, so no service ends up depending on another.
package httpx

import "net/http"

// Code is the wire vocabulary: the only three ways a request, rather than the system, can be at
// fault. Anything else is a failure and must never be mistaken for an answer.
type Code string

const (
	CodeNotFound     Code = "not_found"
	CodeInvalidInput Code = "invalid_input"
	CodeConflict     Code = "conflict"
)

// Known reports whether this is a code a client may act on. A code it does not recognise is a
// failure of the system, not a domain answer.
func (c Code) Known() bool {
	switch c {
	case CodeNotFound, CodeInvalidInput, CodeConflict:
		return true
	default:
		return false
	}
}

// Status is the HTTP status a code is carried on.
func (c Code) Status() int {
	switch c {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeInvalidInput:
		return http.StatusBadRequest
	case CodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// Envelope is the body of every non-2xx response from a service API, and the only shape a client
// accepts as a domain error. Message is written for a visitor.
type Envelope struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// Error is what a client returns for a well-formed domain failure. Anything else comes back as an
// ordinary wrapped error, which is what keeps "the service is down" from being read as an answer.
type Error struct {
	Code    Code
	Message string
}

func (e *Error) Error() string {
	return string(e.Code) + ": " + e.Message
}
