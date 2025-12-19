//go:build !linux

package wine

import (
	"errors"

	"github.com/rs/zerolog"
)

var ErrNotSupported = errors.New("wine is not supported on this platform")

// PrefixOptions contains configuration for a wine prefix
type PrefixOptions struct {
	PrefixPath string
}

// Prefix manages a wine prefix directory (stub for non-Linux)
type Prefix struct{}

// NewPrefix creates a new Prefix instance (stub - returns error on non-Linux)
func NewPrefix(log *zerolog.Logger, opts PrefixOptions) (*Prefix, error) {
	return nil, ErrNotSupported
}

// Path returns the absolute path to the wine prefix
func (p *Prefix) Path() string {
	return ""
}

// Env returns the environment variables needed for wine commands
func (p *Prefix) Env() []string {
	return nil
}

// EnsurePrefix ensures the wine prefix exists (stub - returns error)
func (p *Prefix) EnsurePrefix() error {
	return ErrNotSupported
}

// CheckWine checks if wine is available in PATH (stub - returns error)
func CheckWine() error {
	return ErrNotSupported
}

// CheckXvfb checks if Xvfb is available in PATH (stub - returns error)
func CheckXvfb() error {
	return ErrNotSupported
}
