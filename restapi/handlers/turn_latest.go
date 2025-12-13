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

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewTurnLatestHandler creates a new handler for getting the latest turn
func NewTurnLatestHandler(log *zerolog.Logger, db *sqlx.DB) *TurnLatestHandler {
	return &TurnLatestHandler{db: db, log: log}
}

// TurnLatestHandler handles GET /sessions/{session_id}/turn/latest
type TurnLatestHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *TurnLatestHandler) handle(
	ctx context.Context, params operations.TurnLatestParams, principal *models.Principal,
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

	// Get the player's session player race to find their player order
	var sessionPlayerRace models.SessionPlayerRaceDB
	sprQuery := sessionPlayerRaceQuery(principal.Subject, sessionID)
	if err := sqlH.Get(&sessionPlayerRace, sprQuery); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrForbidden
		}
		return nil, err
	}

	// Query for the latest session files (highest year)
	query := lastSessionFilesQuery(sessionID)
	var sessionFiles models.SessionFilesDB
	if err := sqlH.Get(&sessionFiles, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("no turns found for session: " + sessionID)
		}
		return nil, err
	}

	turnFiles := sessionFiles.ToTurnFiles(sessionPlayerRace.PlayerOrder)
	return &turnFiles, nil
}

// Handle handles the request
func (h *TurnLatestHandler) Handle(
	params operations.TurnLatestParams, principal *models.Principal,
) middleware.Responder {
	turnFiles, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewTurnLatestForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewTurnLatestNotFound().WithPayload(models.FromError(err))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewTurnLatestOK().WithPayload(turnFiles)
}

func (h *TurnLatestHandler) Authorize(
	sqlH database.SQLHelper, params operations.TurnLatestParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// Only session members can access turn files
	return IsSessionMember(sqlH, principal.Subject, params.SessionID)
}
