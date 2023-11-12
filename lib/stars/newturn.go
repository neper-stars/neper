package stars

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

const (
	SubjectTurnSubmitted  = "Turn.Submitted"
	SubjectTurnNotifyBase = "Turn.Notify." // this will be appended with the sessionID
)

type TurnGenerator struct {
	log    *zerolog.Logger
	runner *Runner

	natsConn *nats.Conn
}

func NewTurnGenerator(log *zerolog.Logger, natsConn *nats.Conn, runner *Runner) *TurnGenerator {
	return &TurnGenerator{
		log:      log,
		runner:   runner,
		natsConn: natsConn,
	}
}

// Run is intended to be executed in a goroutine
// when you want to shut down the TurnGenerator.Run goroutine
// just cancel the context you passed in the Run command
func (g *TurnGenerator) Run(ctx context.Context) {
	turnSubmittedChan := make(chan *nats.Msg)

	// we want to listen on the SubjectTurnSubmitted and SubjectTurnWant subjects
	turnSubmittedSub, err := g.natsConn.ChanSubscribe(SubjectTurnSubmitted, turnSubmittedChan)
	if err != nil {
		g.log.Err(err).Str("subject", SubjectTurnSubmitted).Msg("failed to subscribe to subject")
		return
	}
	defer func() {
		if err := turnSubmittedSub.Unsubscribe(); err != nil {
			g.log.Err(err).Str("subject", SubjectTurnSubmitted).Msg("failed to unsubscribe from subject")
		}
	}()
	for {
		select {
		case <-ctx.Done():
			// context is done, goodbye cruel world.
			return
		case msg := <-turnSubmittedChan:
			g.log.Debug().Str("data", string(msg.Data)).Msg("turnSubmitted message received")
		}
	}

}
