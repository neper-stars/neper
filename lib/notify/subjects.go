package notify

// NATS subjects for resource change notifications
const (
	// SubjectResourceChanges is the subject where all resource changes are published
	SubjectResourceChanges = "neper.resource.changes"
)

// Resource types
const (
	TypeSession           = "session"
	TypeInvitation        = "invitation"
	TypeRace              = "race"
	TypeRuleset           = "ruleset"
	TypeSessionPlayerRace = "session_player_race"
)

// Actions
const (
	ActionCreated = "created"
	ActionUpdated = "updated"
	ActionDeleted = "deleted"
)
