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

func (ch *ConnHandlers) ConnErrHandler(conn *nats.Conn, err error) {
	ch.logger.Err(err).
		Str("connStatus", conn.Status().String()).
		Msg("nats client failed to connect")
}

func (ch *ConnHandlers) ErrHandler(conn *nats.Conn, sub *nats.Subscription, err error) {
	ch.logger.Err(err).
		Str("connStatus", conn.Status().String()).
		Str("subject", sub.Subject).
		Msg("nats client error")
}
