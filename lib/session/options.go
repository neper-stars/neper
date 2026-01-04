package session

// Options holds session-related configuration
type Options struct {
	MembershipLimit int `long:"session-membership-limit" env:"SESSION_MEMBERSHIP_LIMIT" ini-name:"session-membership-limit" description:"Maximum number of sessions a user can be part of (as member or manager)" default:"10"`
}

// NewOptions creates a new Options with defaults
func NewOptions() *Options {
	return &Options{
		MembershipLimit: 10,
	}
}
