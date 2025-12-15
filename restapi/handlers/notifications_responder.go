package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	jsoniter "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/zenazn/goji/web/mutil"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
)

const (
	// Time allowed to write a message to the peer.
	notifyWriteWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	notifyPongWait = 90 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	notifyPingPeriod = 30 * time.Second
)

var upgrader = websocket.Upgrader{} // Use default options

// NewNotificationsResponder creates a responder that upgrades to WebSocket and streams notifications
func NewNotificationsResponder(
	req *http.Request,
	db *sqlx.DB,
	natsConn *nats.Conn,
	principal *models.Principal,
) (*NotificationsResponder, error) {
	changeChan := make(chan *nats.Msg, 100)
	sub, err := natsConn.ChanSubscribe(notify.SubjectResourceChanges, changeChan)
	if err != nil {
		return nil, err
	}
	ctx := req.Context()
	logger := zerolog.Ctx(ctx)
	return &NotificationsResponder{
		db:         db,
		req:        req,
		natsSub:    sub,
		changeChan: changeChan,
		logger:     logger,
		principal:  principal,
	}, nil
}

// NotificationsResponder handles WebSocket upgrade and streams resource change notifications
type NotificationsResponder struct {
	db         *sqlx.DB
	req        *http.Request
	natsSub    *nats.Subscription
	changeChan chan *nats.Msg
	logger     *zerolog.Logger
	principal  *models.Principal
}

func (r *NotificationsResponder) Shutdown() {
	if err := r.natsSub.Unsubscribe(); err != nil {
		r.logger.Err(err).Msg("failed to unsubscribe from nats subject")
	}
}

func (r *NotificationsResponder) Logger() *zerolog.Logger {
	return zerolog.Ctx(r.req.Context())
}

// WriteResponse upgrades the connection to WebSocket and streams notifications
func (r *NotificationsResponder) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
	ctx := r.req.Context()

	// Unwrap the response writer to get to the Hijacker interface
	rw_ := rw.(mutil.WriterProxy)
	c, err := upgrader.Upgrade(rw_.Unwrap(), r.req, nil)
	if err != nil {
		r.logger.Err(err).Msg("notifications responder failed to upgrade connection to websocket")
		return
	}
	defer func() {
		if err := c.Close(); err != nil {
			r.logger.Err(err).Msg("failed to close connection")
		}
		r.Shutdown()
	}()

	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	// Set up pong handler to extend read deadline when pong is received
	_ = c.SetReadDeadline(time.Now().Add(notifyPongWait))
	c.SetPongHandler(func(string) error {
		_ = c.SetReadDeadline(time.Now().Add(notifyPongWait))
		return nil
	})

	// Start a goroutine to handle incoming messages (for close detection)
	go r.handleIncoming(ctx, c, ctxCancel)

	pingTicker := time.NewTicker(notifyPingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pingTicker.C:
			_ = c.SetWriteDeadline(time.Now().Add(notifyWriteWait))
			if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
				r.logger.Err(err).Msg("failed to send a ping to the ws client")
				return
			}
		case msg := <-r.changeChan:
			var change notify.ResourceChange
			if err := jsoniter.Unmarshal(msg.Data, &change); err != nil {
				r.logger.Err(err).Msg("failed to unmarshal resource change from nats message")
				continue
			}

			// Check if user has access to this resource
			canAccess, err := r.canAccess(ctx, &change)
			if err != nil {
				r.logger.Err(err).
					Str("type", change.Type).
					Str("id", change.ID).
					Msg("failed to check access for resource change")
				continue
			}
			if !canAccess {
				continue
			}

			// Send the notification to the client
			_ = c.SetWriteDeadline(time.Now().Add(notifyWriteWait))
			if err := c.WriteMessage(websocket.TextMessage, msg.Data); err != nil {
				r.logger.Err(err).Msg("failed to send notification into websocket")
				return
			}
			r.logger.Debug().
				Str("type", change.Type).
				Str("id", change.ID).
				Str("action", change.Action).
				Msg("sent resource change notification")
		}
	}
}

// handleIncoming reads from the WebSocket to detect client disconnect
func (r *NotificationsResponder) handleIncoming(ctx context.Context, c *websocket.Conn, cancel context.CancelFunc) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, _, err := c.ReadMessage()
			if err != nil {
				// Only log if it's an unexpected close error
				// Normal close (1000) and going away (1001) are expected
				if websocket.IsUnexpectedCloseError(err,
					websocket.CloseNormalClosure,
					websocket.CloseGoingAway,
					websocket.CloseNoStatusReceived,
				) {
					r.logger.Err(err).Msg("unexpected websocket close")
				}
				return
			}
		}
	}
}

// canAccess checks if the current user has access to the resource in the notification
func (r *NotificationsResponder) canAccess(ctx context.Context, change *notify.ResourceChange) (bool, error) {
	// Global managers can see everything
	if r.principal.IsGlobalManager {
		return true, nil
	}

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, r.db)
	if err != nil {
		return false, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	switch change.Type {
	case notify.TypeSession:
		return r.canAccessSession(sqlH, change.ID)
	case notify.TypeInvitation:
		return r.canAccessInvitation(sqlH, change)
	case notify.TypeRace:
		return r.canAccessRace(sqlH, change.ID)
	case notify.TypeRuleset:
		return r.canAccessRuleset(sqlH, change.ID)
	case notify.TypeSessionPlayerRace:
		return r.canAccessSessionPlayerRace(sqlH, change.ID)
	case notify.TypeSessionTurn:
		// Session turn access is based on session membership
		return r.canAccessSession(sqlH, change.ID)
	case notify.TypeOrderStatus:
		// Order status access is based on session membership
		return r.canAccessSession(sqlH, change.ID)
	default:
		return false, nil
	}
}

// canAccessSession checks if user can see a session (member or public session)
func (r *NotificationsResponder) canAccessSession(sqlH database.SQLHelper, sessionID string) (bool, error) {
	// Check if user is member
	isMember, err := IsSessionMember(sqlH, r.principal.Subject, sessionID)
	if err != nil {
		return false, err
	}
	if isMember {
		return true, nil
	}

	// Check if session is public
	var session models.SessionDB
	if err := sqlH.GetWhere(&session, sq.Eq{models.SessionDBIDColumn: sessionID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return !session.Private, nil
}

// canAccessInvitation checks if invitation is for this user
func (r *NotificationsResponder) canAccessInvitation(sqlH database.SQLHelper, change *notify.ResourceChange) (bool, error) {
	// For deleted invitations, the record is gone so we check metadata
	if change.Action == notify.ActionDeleted {
		if change.Metadata != nil {
			if userProfileID, ok := change.Metadata["user_profile_id"].(string); ok {
				return userProfileID == r.principal.Subject, nil
			}
		}
		// No metadata available, can't determine access
		return false, nil
	}

	// For create/update, look up the invitation in the database
	var invitation models.InvitationDB
	if err := sqlH.GetWhere(&invitation, sq.Eq{models.InvitationDBIDColumn: change.ID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return invitation.UserProfileID == r.principal.Subject, nil
}

// canAccessRace checks if race belongs to this user
func (r *NotificationsResponder) canAccessRace(sqlH database.SQLHelper, raceID string) (bool, error) {
	var race models.RaceDB
	if err := sqlH.GetWhere(&race, sq.Eq{models.RaceDBIDColumn: raceID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return race.UserID == r.principal.Subject, nil
}

// canAccessRuleset checks if user is member of the session this ruleset belongs to
func (r *NotificationsResponder) canAccessRuleset(sqlH database.SQLHelper, rulesetID string) (bool, error) {
	// Find the session that has this ruleset
	var session models.SessionDB
	if err := sqlH.GetWhere(&session, sq.Eq{models.SessionDBRulesetIDColumn: rulesetID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	// Check if user can access this session
	return r.canAccessSession(sqlH, session.ID)
}

// canAccessSessionPlayerRace checks if user is member of the session
func (r *NotificationsResponder) canAccessSessionPlayerRace(sqlH database.SQLHelper, sprID string) (bool, error) {
	var spr models.SessionPlayerRaceDB
	if err := sqlH.GetWhere(&spr, sq.Eq{models.SessionPlayerRaceDBIDColumn: sprID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	// Check if user is member of the session
	return r.canAccessSession(sqlH, spr.SessionID)
}
