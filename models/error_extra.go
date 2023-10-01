package models

import (
	"fmt"
)

// FromError builds an Error from an error interface
func FromError(err error) *Error {
	if e, ok := err.(*Error); ok { // nolint:errorlint
		return e
	}
	msg := err.Error()
	return &Error{
		Message: &msg,
	}
}

// NewError builds a Error
func NewError(code int64, message string) *Error {
	return &Error{code, &message}
}

// NewErrorf builds a Error from a format string and args
func NewErrorf(code int64, format string, args ...interface{}) *Error {
	message := fmt.Sprintf(format, args...)
	return &Error{code, &message}
}

func (m Error) Error() string {
	if m.Message != nil {
		return fmt.Sprintf("%d %s", m.Code, *m.Message)
	}
	return fmt.Sprintf("%d", m.Code)
}
