package notify

import (
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

// SubjectResourceChanges is the NATS subject where all resource changes are published
const SubjectResourceChanges = "neper.resource.changes"

// Service handles publishing resource change notifications to NATS
type Service struct {
	natsConn *nats.Conn
	log      *zerolog.Logger
}

// NewService creates a new notification service
func NewService(natsConn *nats.Conn, log *zerolog.Logger) *Service {
	return &Service{
		natsConn: natsConn,
		log:      log,
	}
}

// Publish sends a resource change notification to NATS
func (s *Service) Publish(resourceType, resourceID, action string) error {
	timestamp := time.Now().Unix()
	change := ResourceChange{
		Type:      &resourceType,
		ID:        &resourceID,
		Action:    &action,
		Timestamp: &timestamp,
	}

	data, err := jsoniter.Marshal(change)
	if err != nil {
		s.log.Err(err).
			Str("type", resourceType).
			Str("id", resourceID).
			Str("action", action).
			Msg("failed to marshal resource change notification")
		return err
	}

	if err := s.natsConn.Publish(SubjectResourceChanges, data); err != nil {
		s.log.Err(err).
			Str("type", resourceType).
			Str("id", resourceID).
			Str("action", action).
			Msg("failed to publish resource change notification")
		return err
	}

	s.log.Debug().
		Str("type", resourceType).
		Str("id", resourceID).
		Str("action", action).
		Msg("published resource change notification")

	return nil
}

// PublishSessionUpdate is a convenience method for session updates
func (s *Service) PublishSessionUpdate(sessionID string) error {
	return s.Publish(ResourceChangeTypeSession, sessionID, ResourceChangeActionUpdated)
}

// PublishSessionCreate is a convenience method for session creation
func (s *Service) PublishSessionCreate(sessionID string) error {
	return s.Publish(ResourceChangeTypeSession, sessionID, ResourceChangeActionCreated)
}

// PublishSessionDelete is a convenience method for session deletion
// memberIDs is the list of user profile IDs who were members of the session
// isPrivate indicates whether the session was private
func (s *Service) PublishSessionDelete(sessionID string, memberIDs []string, isPrivate bool) error {
	return s.PublishWithMetadata(ResourceChangeTypeSession, sessionID, ResourceChangeActionDeleted, SessionDeleteMeta{
		MemberIDs: memberIDs,
		IsPrivate: isPrivate,
	})
}

// PublishSessionMemberLeft is a convenience method for when a member leaves a session
// leftUserID is the user who left, isPrivate indicates whether the session is private
func (s *Service) PublishSessionMemberLeft(sessionID, leftUserID string, isPrivate bool) error {
	return s.PublishWithMetadata(ResourceChangeTypeSession, sessionID, ResourceChangeActionMemberLeft, SessionMemberLeftMeta{
		LeftUserID: leftUserID,
		IsPrivate:  isPrivate,
	})
}

// PublishInvitationCreate is a convenience method for invitation creation
func (s *Service) PublishInvitationCreate(invitationID string) error {
	return s.Publish(ResourceChangeTypeInvitation, invitationID, ResourceChangeActionCreated)
}

// PublishInvitationDelete is a convenience method for invitation deletion
// userProfileID is included in metadata since the invitation record is deleted before notification
func (s *Service) PublishInvitationDelete(invitationID, userProfileID string) error {
	return s.PublishWithMetadata(ResourceChangeTypeInvitation, invitationID, ResourceChangeActionDeleted, InvitationDeleteMeta{
		UserProfileID: userProfileID,
	})
}

// PublishRaceCreate is a convenience method for race creation
func (s *Service) PublishRaceCreate(raceID string) error {
	return s.Publish(ResourceChangeTypeRace, raceID, ResourceChangeActionCreated)
}

// PublishRaceUpdate is a convenience method for race update
func (s *Service) PublishRaceUpdate(raceID string) error {
	return s.Publish(ResourceChangeTypeRace, raceID, ResourceChangeActionUpdated)
}

// PublishRaceDelete is a convenience method for race deletion
func (s *Service) PublishRaceDelete(raceID string) error {
	return s.Publish(ResourceChangeTypeRace, raceID, ResourceChangeActionDeleted)
}

// PublishSessionPlayerRaceCreate is a convenience method for session player race creation
func (s *Service) PublishSessionPlayerRaceCreate(sprID string) error {
	return s.Publish(ResourceChangeTypeSessionPlayerRace, sprID, ResourceChangeActionCreated)
}

// PublishSessionPlayerRaceUpdate is a convenience method for session player race update
func (s *Service) PublishSessionPlayerRaceUpdate(sprID string) error {
	return s.Publish(ResourceChangeTypeSessionPlayerRace, sprID, ResourceChangeActionUpdated)
}

// PublishSessionPlayerRaceDelete is a convenience method for session player race deletion
func (s *Service) PublishSessionPlayerRaceDelete(sprID string) error {
	return s.Publish(ResourceChangeTypeSessionPlayerRace, sprID, ResourceChangeActionDeleted)
}

// PublishWithMetadata sends a resource change notification with additional metadata
func (s *Service) PublishWithMetadata(resourceType, resourceID, action string, metadata any) error {
	timestamp := time.Now().Unix()
	change := ResourceChange{
		Type:      &resourceType,
		ID:        &resourceID,
		Action:    &action,
		Timestamp: &timestamp,
		Metadata:  metadata,
	}

	data, err := jsoniter.Marshal(change)
	if err != nil {
		s.log.Err(err).
			Str("type", resourceType).
			Str("id", resourceID).
			Str("action", action).
			Msg("failed to marshal resource change notification with metadata")
		return err
	}

	if err := s.natsConn.Publish(SubjectResourceChanges, data); err != nil {
		s.log.Err(err).
			Str("type", resourceType).
			Str("id", resourceID).
			Str("action", action).
			Msg("failed to publish resource change notification with metadata")
		return err
	}

	s.log.Debug().
		Str("type", resourceType).
		Str("id", resourceID).
		Str("action", action).
		Interface("metadata", metadata).
		Msg("published resource change notification with metadata")

	return nil
}

// PublishSessionTurnReady is a convenience method for session turn ready notification
func (s *Service) PublishSessionTurnReady(sessionID string, year int64) error {
	return s.PublishWithMetadata(ResourceChangeTypeSessionTurn, sessionID, ResourceChangeActionReady, SessionTurnMeta{
		Year: year,
	})
}

// PublishOrderStatusUpdate is a convenience method for order status updates
// This is sent when a player submits their orders for a turn
func (s *Service) PublishOrderStatusUpdate(sessionID string, year int64) error {
	return s.PublishWithMetadata(ResourceChangeTypeOrderStatus, sessionID, ResourceChangeActionUpdated, OrderStatusMeta{
		Year: year,
	})
}

// PublishPlayerControlUpdate is a convenience method for player control change notifications
// This is sent when a player is switched between human and AI control
// aiControlType is nil when switching to human, or the AI type code when switching to AI
func (s *Service) PublishPlayerControlUpdate(sessionID string, playerOrder int64, aiControlType *string) error {
	return s.PublishWithMetadata(ResourceChangeTypePlayerControl, sessionID, ResourceChangeActionUpdated, PlayerControlMeta{
		SessionID:     sessionID,
		PlayerOrder:   playerOrder,
		AIControlType: aiControlType,
	})
}

// PublishPendingRegistrationCreate is a convenience method for new pending registration
func (s *Service) PublishPendingRegistrationCreate(userProfileID string) error {
	return s.Publish(ResourceChangeTypePendingRegistration, userProfileID, ResourceChangeActionCreated)
}

// PublishPendingRegistrationApprove is a convenience method for approved registration
// Sends notification to both admins (to update pending list) and the user (to know they've been approved)
func (s *Service) PublishPendingRegistrationApprove(userProfileID, nickname string) error {
	return s.PublishWithMetadata(ResourceChangeTypePendingRegistration, userProfileID, ResourceChangeActionApproved, RegistrationApprovalMeta{
		UserProfileID: userProfileID,
		Nickname:      nickname,
	})
}

// PublishPendingRegistrationReject is a convenience method for rejected registration
func (s *Service) PublishPendingRegistrationReject(userProfileID string) error {
	return s.Publish(ResourceChangeTypePendingRegistration, userProfileID, ResourceChangeActionRejected)
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
