package errors

import "errors"

// **************************
// generic error definitions
// **************************

// ErrNotFound is defined to be matched inside the functional handlers
// to avoid returning errors to the client
var ErrNotFound = errors.New("generic not found error")

// ErrInvalid is defined to be matched inside the functional handlers
// to filter by error types
var ErrInvalid = errors.New("generic invalid error")

// ErrForbidden ...
var ErrForbidden = errors.New("forbidden")

// ErrConflict (409)
var ErrConflict = errors.New("conflict")

// *********************
// error implementations
// *********************

// ErrSessionNotFound no session found matching the criterions
type ErrSessionNotFound struct {
	GivenMessage string
}

// NewErrSessionNotFound is the constructor to obtain an ErrSessionNotFound error
func NewErrSessionNotFound(msg string) *ErrSessionNotFound {
	return &ErrSessionNotFound{GivenMessage: msg}
}

// Error ...
func (e ErrSessionNotFound) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrSessionNotFound) Is(target error) bool {
	return errors.Is(target, ErrNotFound)
}

// ErrUserProfileNotFound no event found matching the criterions
type ErrUserProfileNotFound struct {
	GivenMessage string
}

// NewErrUserProfileNotFound is the constructor to obtain an ErrUserProfileNotFound error
func NewErrUserProfileNotFound(msg string) *ErrUserProfileNotFound {
	return &ErrUserProfileNotFound{GivenMessage: msg}
}

// Error ...
func (e ErrUserProfileNotFound) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrUserProfileNotFound) Is(target error) bool {
	return errors.Is(target, ErrNotFound)
}

// ErrRaceNotFound no event found matching the criterions
type ErrRaceNotFound struct {
	GivenMessage string
}

// NewErrRaceNotFound is the constructor to obtain an ErrRaceNotFound error
func NewErrRaceNotFound(msg string) *ErrRaceNotFound {
	return &ErrRaceNotFound{GivenMessage: msg}
}

// Error ...
func (e ErrRaceNotFound) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrRaceNotFound) Is(target error) bool {
	return errors.Is(target, ErrNotFound)
}

// Invalid Errors

// ErrInvalidAccessLevel ...
type ErrInvalidAccessLevel struct {
	GivenMessage string
}

// NewErrInvalidAccessLevel ...
func NewErrInvalidAccessLevel(msg string) *ErrInvalidAccessLevel {
	return &ErrInvalidAccessLevel{GivenMessage: msg}
}

// Error ...
func (e ErrInvalidAccessLevel) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrInvalidAccessLevel) Is(target error) bool {
	return errors.Is(target, ErrInvalid)
}

// ErrInvalidSession ...
type ErrInvalidSession struct {
	GivenMessage string
}

// NewErrInvalidSession ...
func NewErrInvalidSession(msg string) *ErrInvalidSession {
	return &ErrInvalidSession{GivenMessage: msg}
}

// Error ...
func (e ErrInvalidSession) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrInvalidSession) Is(target error) bool {
	return errors.Is(target, ErrInvalid)
}

// ErrInvalidMember ...
type ErrInvalidMember struct {
	GivenMessage string
}

// NewErrInvalidMember ...
func NewErrInvalidMember(msg string) *ErrInvalidMember {
	return &ErrInvalidMember{GivenMessage: msg}
}

// Error ...
func (e ErrInvalidMember) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrInvalidMember) Is(target error) bool {
	return errors.Is(target, ErrInvalid)
}

// ErrAlreadyExists ...
type ErrAlreadyExists struct {
	GivenMessage string
}

// NewErrAlreadyExists is the constructor to obtain an ErrAlreadyExists error
func NewErrAlreadyExists(msg string) *ErrAlreadyExists {
	return &ErrAlreadyExists{GivenMessage: msg}
}

// Error ...
func (e ErrAlreadyExists) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrAlreadyExists) Is(target error) bool {
	// am I an ErrConflict ? Yes
	return errors.Is(target, ErrConflict)
}

// ErrInvalidRace ...
type ErrInvalidRace struct {
	GivenMessage string
}

// NewErrInvalidRace ...
func NewErrInvalidRace(msg string) *ErrInvalidRace {
	return &ErrInvalidRace{GivenMessage: msg}
}

// Error ...
func (e ErrInvalidRace) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrInvalidRace) Is(target error) bool {
	return errors.Is(target, ErrInvalid)
}
