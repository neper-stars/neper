package sync

// Operation is the type of change
type Operation int

const (
	// OpCreate is when a record is created
	OpCreate Operation = 1

	// OpUpdate is when a record is updated
	OpUpdate Operation = 2

	// OpNoChange is when a record did not change
	OpNoChange Operation = 3

	// NeperReferentialBroadcast is one of the messages that can be received
	NeperReferentialBroadcast = "neper-referential-broadcast"
)
