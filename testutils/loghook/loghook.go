package loghook

import (
	"sync"

	"github.com/rs/zerolog"
)

// Entry captures a single log event for testing.
type Entry struct {
	Level   zerolog.Level
	Message string
}

// CaptureHook captures log events for assertions in tests.
// Use it with zerolog's Hook method to intercept log messages.
//
// Example:
//
//	hook := &loghook.CaptureHook{}
//	log := testutils.GetLogger(t).Hook(hook)
//	// ... perform operations that log ...
//	require.True(t, hook.HasMessage(zerolog.WarnLevel, "expected warning"))
type CaptureHook struct {
	mu      sync.Mutex
	entries []Entry
}

// Run implements zerolog.Hook interface.
func (h *CaptureHook) Run(e *zerolog.Event, level zerolog.Level, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, Entry{Level: level, Message: message})
}

// HasMessage returns true if a log entry with the given level and message exists.
func (h *CaptureHook) HasMessage(level zerolog.Level, message string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range h.entries {
		if e.Level == level && e.Message == message {
			return true
		}
	}
	return false
}

// Entries returns a copy of all captured log entries.
func (h *CaptureHook) Entries() []Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]Entry, len(h.entries))
	copy(result, h.entries)
	return result
}

// Clear removes all captured log entries.
func (h *CaptureHook) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = nil
}
