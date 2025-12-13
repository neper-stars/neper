package notify

import (
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

// ResourceChange represents a notification about a resource modification
type ResourceChange struct {
	Type      string `json:"type"`      // Resource type: "session", "invitation", etc.
	ID        string `json:"id"`        // Resource ID
	Action    string `json:"action"`    // Action: "created", "updated", "deleted"
	Timestamp int64  `json:"timestamp"` // Unix timestamp
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
func (s *Service) PublishSessionDelete(sessionID string) error {
	return s.Publish(TypeSession, sessionID, ActionDeleted)
}

// PublishInvitationCreate is a convenience method for invitation creation
func (s *Service) PublishInvitationCreate(invitationID string) error {
	return s.Publish(TypeInvitation, invitationID, ActionCreated)
}

// PublishInvitationDelete is a convenience method for invitation deletion
func (s *Service) PublishInvitationDelete(invitationID string) error {
	return s.Publish(TypeInvitation, invitationID, ActionDeleted)
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
