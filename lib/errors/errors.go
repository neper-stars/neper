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

// ErrPreconditionFailed (412)
var ErrPreconditionFailed = errors.New("precondition failed")

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

// ErrSomethingNotFound is an error when you search something and don't find it.
// it will match the Base ErrNotFound
type ErrSomethingNotFound struct {
	GivenMessage string
}

// NewErrSomethingNotFound is the constructor to obtain an ErrSessionNotFound error
func NewErrSomethingNotFound(msg string) *ErrSessionNotFound {
	return &ErrSessionNotFound{GivenMessage: msg}
}

// Error ...
func (e ErrSomethingNotFound) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrSomethingNotFound) Is(target error) bool {
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

// Rules

// ErrRulesNotFound no event found matching the criterions
type ErrRulesNotFound struct {
	GivenMessage string
}

// NewErrRulesNotFound is the constructor to obtain an ErrRaceNotFound error
func NewErrRulesNotFound(msg string) *ErrRulesNotFound {
	return &ErrRulesNotFound{GivenMessage: msg}
}

// Error ...
func (e ErrRulesNotFound) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrRulesNotFound) Is(target error) bool {
	return errors.Is(target, ErrNotFound)
}

// Race error

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

// ErrInvalidSomething ...
type ErrInvalidSomething struct {
	GivenMessage string
}

// NewErrInvalidSomething ...
func NewErrInvalidSomething(msg string) *ErrInvalidSomething {
	return &ErrInvalidSomething{GivenMessage: msg}
}

// Error ...
func (e ErrInvalidSomething) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrInvalidSomething) Is(target error) bool {
	return errors.Is(target, ErrInvalid)
}

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

// ErrRaceInUse ...
type ErrRaceInUse struct {
	GivenMessage string
}

// NewErrRaceInUse ...
func NewErrRaceInUse(msg string) *ErrRaceInUse {
	return &ErrRaceInUse{GivenMessage: msg}
}

// Error ...
func (e ErrRaceInUse) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrRaceInUse) Is(target error) bool {
	return errors.Is(target, ErrConflict)
}

// ErrPlayersNotReady ...
type ErrPlayersNotReady struct {
	GivenMessage string
}

// NewErrPlayersNotReady ...
func NewErrPlayersNotReady(msg string) *ErrPlayersNotReady {
	return &ErrPlayersNotReady{GivenMessage: msg}
}

// Error ...
func (e ErrPlayersNotReady) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrPlayersNotReady) Is(target error) bool {
	return errors.Is(target, ErrPreconditionFailed)
}

// ErrSessionAlreadyStarted ...
type ErrSessionAlreadyStarted struct {
	GivenMessage string
}

// NewErrSessionAlreadyStarted ...
func NewErrSessionAlreadyStarted(msg string) *ErrSessionAlreadyStarted {
	return &ErrSessionAlreadyStarted{GivenMessage: msg}
}

// Error ...
func (e ErrSessionAlreadyStarted) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrSessionAlreadyStarted) Is(target error) bool {
	return errors.Is(target, ErrPreconditionFailed)
}

// ErrLastManager is returned when a manager tries to leave a session
// but they are the only manager
type ErrLastManager struct {
	GivenMessage string
}

// NewErrLastManager ...
func NewErrLastManager(msg string) *ErrLastManager {
	return &ErrLastManager{GivenMessage: msg}
}

// Error ...
func (e ErrLastManager) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrLastManager) Is(target error) bool {
	return errors.Is(target, ErrPreconditionFailed)
}

// ErrNotAMember is returned when an operation requires the target user
// to be a member of the session but they are not
type ErrNotAMember struct {
	GivenMessage string
}

// NewErrNotAMember ...
func NewErrNotAMember(msg string) *ErrNotAMember {
	return &ErrNotAMember{GivenMessage: msg}
}

// Error ...
func (e ErrNotAMember) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrNotAMember) Is(target error) bool {
	return errors.Is(target, ErrPreconditionFailed)
}

// ErrSessionNotStarted is returned when an operation requires the session
// to be started but it is not
type ErrSessionNotStarted struct {
	GivenMessage string
}

// NewErrSessionNotStarted ...
func NewErrSessionNotStarted(msg string) *ErrSessionNotStarted {
	return &ErrSessionNotStarted{GivenMessage: msg}
}

// Error ...
func (e ErrSessionNotStarted) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrSessionNotStarted) Is(target error) bool {
	return errors.Is(target, ErrPreconditionFailed)
}

// ErrPlayerShouldNotBeReady is returned when a player tries to leave a session
// but they have already marked themselves as ready
type ErrPlayerShouldNotBeReady struct {
	GivenMessage string
}

// NewErrPlayerShouldNotBeReady ...
func NewErrPlayerShouldNotBeReady(msg string) *ErrPlayerShouldNotBeReady {
	return &ErrPlayerShouldNotBeReady{GivenMessage: msg}
}

// Error ...
func (e ErrPlayerShouldNotBeReady) Error() string {
	return e.GivenMessage
}

// Is ...
func (e ErrPlayerShouldNotBeReady) Is(target error) bool {
	return errors.Is(target, ErrPreconditionFailed)
}
