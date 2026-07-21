package inference

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorInvalidRequest ErrorKind = "invalid_request"
	ErrorAuthentication ErrorKind = "authentication"
	ErrorRateLimited    ErrorKind = "rate_limited"
	ErrorUnavailable    ErrorKind = "unavailable"
	ErrorTimeout        ErrorKind = "timeout"
	ErrorProvider       ErrorKind = "provider_error"
	ErrorInvalidOutput  ErrorKind = "invalid_output"
)

type Error struct {
	Kind       ErrorKind
	Provider   string
	StatusCode int
	Retryable  bool
	Message    string
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	prefix := string(e.Kind)
	if e.Provider != "" {
		prefix = e.Provider + ": " + prefix
	}
	if e.Message != "" {
		return prefix + ": " + e.Message
	}
	return prefix
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(kind ErrorKind, provider, message string, retryable bool, cause error) error {
	return &Error{Kind: kind, Provider: provider, Message: message, Retryable: retryable, Cause: cause}
}

func IsKind(err error, kind ErrorKind) bool {
	var target *Error
	return errors.As(err, &target) && target.Kind == kind
}

func ResponseError(provider string, status int, message string) error {
	kind, retryable := ErrorProvider, false
	switch {
	case status == 401 || status == 403:
		kind = ErrorAuthentication
	case status == 429:
		kind, retryable = ErrorRateLimited, true
	case status >= 500:
		kind, retryable = ErrorUnavailable, true
	}
	return &Error{Kind: kind, Provider: provider, StatusCode: status, Retryable: retryable, Message: fmt.Sprintf("status=%d body=%s", status, message)}
}
