package yammy

import (
	"errors"
	"fmt"
)

var _ error = (*wrappedError)(nil)

// Err is an error occurrs in yammy.
var Err = defineError("yammy error", nil)

// ErrIO is an error ralated to io.
var ErrIO = defineError("io error", Err)

// ErrYAML is an error related to YAML parsing.
var ErrYAML = defineError("yaml error", Err)

// ErrDirective is an error related to yammy directives.
var ErrDirective = defineError("directive error", Err)

// ErrVarNotFound is an error that means variables used in a
// YAML not found.
var ErrVarNotFound = defineError("variable not found", Err)

type wrappedError struct {
	message string
	parent  *wrappedError
	cause   error
}

func defineError(message string, parent *wrappedError) *wrappedError {
	return &wrappedError{
		message: message,
		parent:  parent,
	}
}

func (e *wrappedError) New(message string, cause error, args ...any) *wrappedError {
	if len(args) != 0 {
		message = fmt.Sprintf(message, args...)
	}
	return &wrappedError{
		message: message,
		parent:  e,
		cause:   cause,
	}
}

func (e *wrappedError) Is(other error) bool {
	return e != nil && (e == other || errors.Is(e.parent, other) || errors.Is(e.cause, other))
}

func (e *wrappedError) Error() string {
	ret := e.message
	if e.cause != nil {
		ret = fmt.Errorf("%s: %w", e.message, e.cause).Error()
	}
	if e.parent != nil {
		ret = e.parent.message + ": " + ret
		for p := e.parent.parent; p != nil; p = p.parent {
			ret = ret + ": " + p.message
		}
	}

	return ret
}

func (e *wrappedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
