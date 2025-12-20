package notify

import (
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

// ResourceChange represents a notification about a resource modification
type ResourceChange struct {
	Type      string `json:"type"`               // Resource type: "session", "invitation", etc.
	ID        string `json:"id"`                 // Resource ID
	Action    string `json:"action"`             // Action: "created", "updated", "deleted", "ready"
	Timestamp int64  `json:"timestamp"`          // Unix timestamp
	Metadata  any    `json:"metadata,omitempty"` // Optional resource-specific data (typed based on Type)
}

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
	change := ResourceChange{
		Type:      resourceType,
		ID:        resourceID,
		Action:    action,
		Timestamp: time.Now().Unix(),
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
	return s.Publish(TypeSession, sessionID, ActionUpdated)
}

// PublishSessionCreate is a convenience method for session creation
func (s *Service) PublishSessionCreate(sessionID string) error {
	return s.Publish(TypeSession, sessionID, ActionCreated)
}

// PublishSessionDelete is a convenience method for session deletion
// memberIDs is the list of user profile IDs who were members of the session
// isPrivate indicates whether the session was private
func (s *Service) PublishSessionDelete(sessionID string, memberIDs []string, isPrivate bool) error {
	return s.PublishWithMetadata(TypeSession, sessionID, ActionDeleted, SessionDeleteMeta{
		MemberIDs: memberIDs,
		IsPrivate: isPrivate,
	})
}

// PublishSessionMemberLeft is a convenience method for when a member leaves a session
// leftUserID is the user who left, isPrivate indicates whether the session is private
func (s *Service) PublishSessionMemberLeft(sessionID, leftUserID string, isPrivate bool) error {
	return s.PublishWithMetadata(TypeSession, sessionID, ActionMemberLeft, SessionMemberLeftMeta{
		LeftUserID: leftUserID,
		IsPrivate:  isPrivate,
	})
}

// PublishInvitationCreate is a convenience method for invitation creation
func (s *Service) PublishInvitationCreate(invitationID string) error {
	return s.Publish(TypeInvitation, invitationID, ActionCreated)
}

// PublishInvitationDelete is a convenience method for invitation deletion
// userProfileID is included in metadata since the invitation record is deleted before notification
func (s *Service) PublishInvitationDelete(invitationID, userProfileID string) error {
	return s.PublishWithMetadata(TypeInvitation, invitationID, ActionDeleted, InvitationDeleteMeta{
		UserProfileID: userProfileID,
	})
}

// PublishRaceCreate is a convenience method for race creation
func (s *Service) PublishRaceCreate(raceID string) error {
	return s.Publish(TypeRace, raceID, ActionCreated)
}

// PublishRaceUpdate is a convenience method for race update
func (s *Service) PublishRaceUpdate(raceID string) error {
	return s.Publish(TypeRace, raceID, ActionUpdated)
}

// PublishRaceDelete is a convenience method for race deletion
func (s *Service) PublishRaceDelete(raceID string) error {
	return s.Publish(TypeRace, raceID, ActionDeleted)
}

// PublishSessionPlayerRaceCreate is a convenience method for session player race creation
func (s *Service) PublishSessionPlayerRaceCreate(sprID string) error {
	return s.Publish(TypeSessionPlayerRace, sprID, ActionCreated)
}

// PublishSessionPlayerRaceUpdate is a convenience method for session player race update
func (s *Service) PublishSessionPlayerRaceUpdate(sprID string) error {
	return s.Publish(TypeSessionPlayerRace, sprID, ActionUpdated)
}

// PublishWithMetadata sends a resource change notification with additional metadata
func (s *Service) PublishWithMetadata(resourceType, resourceID, action string, metadata any) error {
	change := ResourceChange{
		Type:      resourceType,
		ID:        resourceID,
		Action:    action,
		Timestamp: time.Now().Unix(),
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
	return s.PublishWithMetadata(TypeSessionTurn, sessionID, ActionReady, SessionTurnMeta{
		Year: year,
	})
}

// PublishOrderStatusUpdate is a convenience method for order status updates
// This is sent when a player submits their orders for a turn
func (s *Service) PublishOrderStatusUpdate(sessionID string, year int64) error {
	return s.PublishWithMetadata(TypeOrderStatus, sessionID, ActionUpdated, OrderStatusMeta{
		Year: year,
	})
}

// PublishPendingRegistrationCreate is a convenience method for new pending registration
func (s *Service) PublishPendingRegistrationCreate(userProfileID string) error {
	return s.Publish(TypePendingRegistration, userProfileID, ActionCreated)
}

// PublishPendingRegistrationApprove is a convenience method for approved registration
func (s *Service) PublishPendingRegistrationApprove(userProfileID string) error {
	return s.Publish(TypePendingRegistration, userProfileID, ActionApproved)
}

// PublishPendingRegistrationReject is a convenience method for rejected registration
func (s *Service) PublishPendingRegistrationReject(userProfileID string) error {
	return s.Publish(TypePendingRegistration, userProfileID, ActionRejected)
}
