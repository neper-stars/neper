package sessionSubmitter

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/neper-stars/neper/lib/stars"
)

type SessionSubmitter interface {
	Sub(sessionID string, year int64) error
}

// TestSessionSubmitter does not send any message but actually keeps them
// for easy inspection in tests
type TestSessionSubmitter struct {
	log  *zerolog.Logger
	Msgs []*nats.Msg
}

func NewTestSessionSubmitter(log *zerolog.Logger) *TestSessionSubmitter {
	return &TestSessionSubmitter{log: log}
}

func (s *TestSessionSubmitter) Sub(sessionID string, year int64) error {
	msg, err := genMsg(s.log, sessionID, year)
	if err != nil {
		return err
	}
	s.Msgs = append(s.Msgs, msg)
	return nil
}

type ActualSessionSubmitter struct {
	conn *nats.Conn
	log  *zerolog.Logger
}

func NewSessionSubmitter(log *zerolog.Logger, conn *nats.Conn) *ActualSessionSubmitter {
	return &ActualSessionSubmitter{
		log:  log,
		conn: conn,
	}
}

func genMsg(log *zerolog.Logger, sessionID string, year int64) (*nats.Msg, error) {
	msg := nats.NewMsg(stars.SubjectTurnNeedsGeneration)
	needsGenerationMsg := stars.NeedsGenerationMsg{
		SessionID: sessionID,
		Year:      int(year),
	}
	data, err := jsoniter.Marshal(needsGenerationMsg)
	if err != nil {
		log.Err(err).Str("sessionID", sessionID).Msg("failed to marshal the message for the turn generator")
		return nil, err
	}

	msg.Data = data
	return msg, nil
}

func (s *ActualSessionSubmitter) Sub(sessionID string, year int64) error {
	msg, err := genMsg(s.log, sessionID, year)
	if err != nil {
		return err
	}

	if err := s.conn.PublishMsg(msg); err != nil {
		// here we failed to publish our message.
		// This should not be a big issue because in the long run the
		// turn generator should auto find sessions that need generation
		// let's just log this error
		s.log.Err(err).Str("sessionID", sessionID).Msg("failed to signal the turn generator, our session is ready")
	}
	return nil
}
