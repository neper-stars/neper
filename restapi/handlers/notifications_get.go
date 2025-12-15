package handlers

import (
	"net/http"
	"strings"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewNotificationsHandler creates a new notifications handler
func NewNotificationsHandler(log *zerolog.Logger, db *sqlx.DB, natsConn *nats.Conn) *NotificationsHandler {
	configuredLogger := log.With().Str("handler", "NotificationsHandler").Logger()
	return &NotificationsHandler{
		db:       db,
		log:      &configuredLogger,
		natsConn: natsConn,
	}
}

// NotificationsHandler handles /v1/notifications
type NotificationsHandler struct {
	db       *sqlx.DB
	log      *zerolog.Logger
	natsConn *nats.Conn
}

// Handle handles the request
func (h *NotificationsHandler) Handle(
	params operations.NotificationsParams, principal *models.Principal,
) middleware.Responder {
	// Check if client wants WebSocket upgrade
	if !wantsWebSocket(params.HTTPRequest) {
		// Return error - this endpoint only supports WebSocket
		return operations.NewNotificationsDefault(400).WithPayload(&models.Error{
			Code:    400,
			Message: strPtr("This endpoint requires a WebSocket upgrade"),
		})
	}

	// Create and return the WebSocket responder
	responder, err := NewNotificationsResponder(
		params.HTTPRequest,
		h.db,
		h.natsConn,
		principal,
	)
	if err != nil {
		h.log.Err(err).Msg("failed to create notifications responder")
		return InternalError(err, h.log, false)
	}

	return responder
}

func strPtr(s string) *string {
	return &s
}

func wantsWebSocket(r *http.Request) bool {
	var connection string
	var upgrade string

	for k, v := range r.Header {
		if strings.ToLower(k) == "connection" {
			connection = strings.Join(v, "")
		} else if strings.ToLower(k) == "upgrade" {
			upgrade = strings.Join(v, "")
		}
	}
	if strings.ToLower(connection) == "upgrade" && upgrade == "websocket" {
		return true
	}
	return false
}
