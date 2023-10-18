package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	sq "github.com/Masterminds/squirrel"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewTurnGetHandler ...
func NewTurnGetHandler(log *zerolog.Logger, db *sqlx.DB) *TurnGetHandler {
	configuredLogger := log.With().Str("handler", "TurnGetHandler").Logger()
	return &TurnGetHandler{db, &configuredLogger}
}

// TurnGetHandler handles /session
type TurnGetHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *TurnGetHandler) handle(
	ctx context.Context, params operations.TurnGetParams, principal *models.Principal,
) (*models.TurnFiles, error) {
	sessionID := params.SessionID

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** Authorization **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

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
	turnFiles := sessionFiles.ToTurnFiles(userSessionSetup.PlayerOrder)

	return &turnFiles, nil
}

// Handle handles the request
func (h *TurnGetHandler) Handle(
	params operations.TurnGetParams, principal *models.Principal,
) middleware.Responder {
	turn, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewTurnGetForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewTurnGetOK().WithPayload(turn)
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
