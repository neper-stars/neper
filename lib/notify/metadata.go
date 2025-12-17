package notify

import jsoniter "github.com/json-iterator/go"

// SessionDeleteMeta contains metadata for session deletion notifications
type SessionDeleteMeta struct {
	MemberIDs []string `json:"member_ids"`
	IsPrivate bool     `json:"is_private"`
}

// InvitationDeleteMeta contains metadata for invitation deletion notifications
type InvitationDeleteMeta struct {
	UserProfileID string `json:"user_profile_id"`
}

// SessionTurnMeta contains metadata for session turn notifications
type SessionTurnMeta struct {
	Year int64 `json:"year"`
}

// OrderStatusMeta contains metadata for order status notifications
type OrderStatusMeta struct {
	Year int64 `json:"year"`
}

// ParseMetadata parses the metadata from a ResourceChange into a typed struct.
// The target must be a pointer to the expected metadata type.
func ParseMetadata[T any](change *ResourceChange) *T {
	if change.Metadata == nil {
		return nil
	}

	// Re-marshal and unmarshal to convert map[string]any to typed struct
	data, err := jsoniter.Marshal(change.Metadata)
	if err != nil {
		return nil
	}

	var meta T
	if err := jsoniter.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}
