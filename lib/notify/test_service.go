package notify

import (
	"github.com/rs/zerolog"
)

// NotifyService is an interface for publishing notifications.
// It is implemented by both Service and TestNotifyService.
type NotifyService interface {
	PublishPlayerControlUpdate(sessionID string, playerOrder int64, aiControlType *string) error
	PublishOrderStatusUpdate(sessionID string, year int64) error
}

// TestNotifyService does not send any messages but keeps them
// for easy inspection in tests
type TestNotifyService struct {
	log      *zerolog.Logger
	Messages []ResourceChange
}

func NewTestNotifyService(log *zerolog.Logger) *TestNotifyService {
	return &TestNotifyService{log: log}
}

// Publish records a resource change notification for testing
func (s *TestNotifyService) Publish(resourceType, resourceID, action string) error {
	change := ResourceChange{
		Type:   &resourceType,
		ID:     &resourceID,
		Action: &action,
	}
	s.Messages = append(s.Messages, change)
	return nil
}

// PublishWithMetadata records a resource change notification with metadata for testing
func (s *TestNotifyService) PublishWithMetadata(resourceType, resourceID, action string, metadata any) error {
	change := ResourceChange{
		Type:     &resourceType,
		ID:       &resourceID,
		Action:   &action,
		Metadata: metadata,
	}
	s.Messages = append(s.Messages, change)
	return nil
}

// PublishPlayerControlUpdate records a player control change notification
func (s *TestNotifyService) PublishPlayerControlUpdate(sessionID string, playerOrder int64, aiControlType *string) error {
	return s.PublishWithMetadata(ResourceChangeTypePlayerControl, sessionID, ResourceChangeActionUpdated, PlayerControlMeta{
		SessionID:     sessionID,
		PlayerOrder:   playerOrder,
		AIControlType: aiControlType,
	})
}

// PublishOrderStatusUpdate records an order status update notification
func (s *TestNotifyService) PublishOrderStatusUpdate(sessionID string, year int64) error {
	return s.PublishWithMetadata(ResourceChangeTypeOrderStatus, sessionID, ResourceChangeActionUpdated, OrderStatusMeta{
		Year: year,
	})
}

// Clear resets the messages slice
func (s *TestNotifyService) Clear() {
	s.Messages = nil
}

// GetPlayerControlMessages returns only player_control type messages
func (s *TestNotifyService) GetPlayerControlMessages() []ResourceChange {
	var result []ResourceChange
	for _, msg := range s.Messages {
		if msg.Type != nil && *msg.Type == ResourceChangeTypePlayerControl {
			result = append(result, msg)
		}
	}
	return result
}
