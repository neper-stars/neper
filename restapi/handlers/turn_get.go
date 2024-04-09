package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/nats-io/nats.go"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewTurnGetHandler ...
func NewTurnGetHandler(log *zerolog.Logger, db *sqlx.DB, natsConn *nats.Conn) *TurnGetHandler {
	configuredLogger := log.With().Str("handler", "TurnGetHandler").Logger()
	return &TurnGetHandler{db, &configuredLogger, natsConn}
}

// TurnGetHandler handles /session
type TurnGetHandler struct {
	db       *sqlx.DB
	log      *zerolog.Logger
	natsConn *nats.Conn
}

func (h *TurnGetHandler) handle(
	sqlH database.SQLHelper, params operations.TurnGetParams, principal *models.Principal, playerOrder int64,
) (*models.TurnFiles, error) {
	sessionID := params.SessionID

	var sessionFiles models.SessionFilesDB
	whereClause := sq.And{
		sq.Eq{models.SessionFilesDBSessionIDColumn: sessionID},
		sq.Eq{models.SessionFilesDBYearColumn: params.Year},
	}

	if err := sqlH.GetWhere(&sessionFiles, whereClause); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("session files not found with session ID: " + sessionID)
		}
		return nil, err
	}

	var userSessionSetup models.SessionPlayerRaceDB
	query := sessionPlayerRaceQuery(principal.Subject, params.SessionID)
	if err := sqlH.Get(&userSessionSetup, query); err != nil {
		h.log.Err(err).Msg("failed to fetch user profile session relation")
		return nil, err
	}
	turnFiles := sessionFiles.ToTurnFiles(playerOrder)

	return &turnFiles, nil
}

func (h *TurnGetHandler) getSessionPlayerRace(sqlH database.SQLHelper, userProfileID, sessionID string) (*models.SessionPlayerRaceDB, error) {
	var sessionPlayerRaceDB models.SessionPlayerRaceDB
	query := sessionPlayerRaceQuery(userProfileID, sessionID)
	if err := sqlH.Get(&sessionPlayerRaceDB, query); err != nil {
		h.log.Err(err).Msg("failed to fetch user profile session relation")
		return nil, err
	}
	return &sessionPlayerRaceDB, nil
}

// Handle handles the request
func (h *TurnGetHandler) Handle(
	params operations.TurnGetParams, principal *models.Principal,
) middleware.Responder {
	wsRequested := wantsWebSocket(params.HTTPRequest)
	ctx := params.HTTPRequest.Context()
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
	}

	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** Authorization **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
	}
	if !authorized {
		return operations.NewTurnGetForbidden().WithPayload(&models.Error{
			Code:    http.StatusForbidden,
			Message: &verbotten,
		})
	}

	spr, err := h.getSessionPlayerRace(sqlH, principal.Subject, params.SessionID)
	if err != nil {
		// TODO: handle the case when the session ID is not found
		// by returning a not found error
		return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
	}

	if wantsWebSocket(params.HTTPRequest) {
		// if the client wants a websocket upgrade let's give it a turn responder
		// this will open a websocket that will push a new turn when it becomes available
		details := TurnDetails{
			SessionID:   params.SessionID,
			Year:        int(params.Year),
			PlayerOrder: spr.PlayerOrder,
		}
		tr, err := NewTurnResponder(params.HTTPRequest, h.db, &details, h.natsConn)
		if err != nil {
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
		return tr
	}

	turn, err := h.handle(sqlH, params, principal, spr.PlayerOrder)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewTurnGetForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound) && !wsRequested:
			// Websocket was not requested and the file is not found...
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewTurnGetOK().WithPayload(turn)
}

func wantsWebSocket(r *http.Request) bool {
	var connection string
	var upgrade string

	for k, v := range r.Header {
		if strings.ToLower(k) == "connection" { // Connection header
			connection = strings.Join(v, "")
		} else if strings.ToLower(k) == "upgrade" { // Upgrade header
			upgrade = strings.Join(v, "")
		}
	}
	if strings.ToLower(connection) == "upgrade" && upgrade == "websocket" {
		return true
	}
	return false
}

func (h *TurnGetHandler) Authorize(
	sqlH database.SQLHelper, params operations.TurnGetParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	var authRes models.UserProfileSessionRelDB
	query := userProfileSessionRelationQuery(principal.Subject, params.SessionID)
	if err := sqlH.Get(&authRes, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.log.Debug().Msg("auth: no matching userprofile session relation --> reject")
			return false, nil
		}
		return false, err
	}
	return true, nil
}
