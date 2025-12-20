package notify

// NATS subjects for resource change notifications
const (
	// SubjectResourceChanges is the subject where all resource changes are published
	SubjectResourceChanges = "neper.resource.changes"
)

// Resource types
const (
	TypeSession             = "session"
	TypeSessionTurn         = "session_turn"
	TypeOrderStatus         = "order_status"
	TypeInvitation          = "invitation"
	TypeRace                = "race"
	TypeRuleset             = "ruleset"
	TypeSessionPlayerRace   = "session_player_race"
	TypePendingRegistration = "pending_registration"
)

// Actions
const (
	ActionCreated    = "created"
	ActionUpdated    = "updated"
	ActionDeleted    = "deleted"
	ActionReady      = "ready"
	ActionMemberLeft = "member_left"
	ActionApproved   = "approved"
	ActionRejected   = "rejected"
)
