package race

// Options holds race-related configuration
type Options struct {
	PendingUserLimit  int `long:"race-limit-pending" env:"RACE_LIMIT_PENDING" ini-name:"race-limit-pending" description:"Maximum number of races a pending user can upload" default:"3"`
	ApprovedUserLimit int `long:"race-limit-approved" env:"RACE_LIMIT_APPROVED" ini-name:"race-limit-approved" description:"Maximum number of races an approved user can upload" default:"50"`
}

// NewOptions creates a new Options with defaults
func NewOptions() *Options {
	return &Options{
		PendingUserLimit:  3,
		ApprovedUserLimit: 50,
	}
}
