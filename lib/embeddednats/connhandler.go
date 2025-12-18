package embeddednats

import (
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

type ConnHandlers struct {
	logger *zerolog.Logger
}

func NewConnHandlers(zl *zerolog.Logger) *ConnHandlers {
	return &ConnHandlers{
		logger: zl,
	}
}

func (ch *ConnHandlers) ConnHandler(conn *nats.Conn) {
	ch.logger.Info().
		Str("connStatus", conn.Status().String()).
		Msg("nats client connected successfully")
}

func (ch *ConnHandlers) DisconnectErrHandler(conn *nats.Conn, err error) {
	// During normal shutdown, err is nil and status is CLOSED - don't log as error
	if err == nil && conn.Status() == nats.CLOSED {
		ch.logger.Debug().
			Str("connStatus", conn.Status().String()).
			Msg("nats client disconnected")
		return
	}
	ch.logger.Err(err).
		Str("connStatus", conn.Status().String()).
		Msg("nats client disconnected unexpectedly")
}

func (ch *ConnHandlers) ErrHandler(conn *nats.Conn, sub *nats.Subscription, err error) {
	ch.logger.Err(err).
		Str("connStatus", conn.Status().String()).
		// sub is not always non-nil ...
		// Str("subject", sub.Subject).
		Msg("nats client error")
}
