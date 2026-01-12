package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/neper-stars/houston/lib/tools/playerchanger"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionPlayerSwitchToHumanHandler creates a new handler for switching a player to human control
func NewSessionPlayerSwitchToHumanHandler(log *zerolog.Logger, db *sqlx.DB) *SessionPlayerSwitchToHumanHandler {
	return &SessionPlayerSwitchToHumanHandler{db: db, log: log}
}

// SessionPlayerSwitchToHumanHandler handles POST /sessions/{session_id}/player/{player_order}/switch_to_human
type SessionPlayerSwitchToHumanHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *SessionPlayerSwitchToHumanHandler) handle(
	ctx context.Context, params operations.SessionPlayerSwitchToHumanParams, principal *models.Principal,
) error {
	sessionID := params.SessionID
	playerOrder := int(params.PlayerOrder)

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** Authorization - managers only **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return err
	}
	if !authorized {
		return errs.ErrForbidden
	}

	// Get the session_player_race entry to verify preconditions
	var spr models.SessionPlayerRaceDB
	query := sessionPlayerRaceByOrderQuery(sessionID, playerOrder)
	if err := sqlH.Get(&spr, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrPreconditionFailed("player not found in session")
		}
		return err
	}

	// Precondition: must be a real player slot (not a bot)
	if spr.IsBot {
		return errs.NewErrPreconditionFailed("cannot switch bot player to human control: no real user associated")
	}

	// Precondition: must currently be AI-controlled
	if spr.AIControlType == nil {
		return errs.NewErrPreconditionFailed("player is already human controlled")
	}

	// Query for the latest session files (highest year)
	filesQuery := lastSessionFilesQuery(sessionID)
	var sessionFilesDB models.SessionFilesDB
	if err := sqlH.Get(&sessionFilesDB, filesQuery); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrPreconditionFailed("game not started: no session files found for session")
		}
		return err
	}

	// Decode the host file from base64
	hstData, err := stars.B64Decode(sessionFilesDB.HostFile)
	if err != nil {
		return errs.NewErrInvalidSomething("failed to decode host file: " + err.Error())
	}

	// Switch the player to human using houston's playerchanger
	modifiedHst, _, err := playerchanger.ChangeToHumanBytes(hstData, playerOrder)
	if err != nil {
		return errs.NewErrInvalidSomething("failed to switch player to human: " + err.Error())
	}

	// Encode the modified host file back to base64
	sessionFilesDB.HostFile = stars.B64Encode(modifiedHst)

	// Update the session files in the database
	if err := sqlH.UpdateColumns(&sessionFilesDB, models.SessionFilesDBHostFileColumn); err != nil {
		return err
	}

	// Clear ai_control_type to mark as human controlled
	spr.AIControlType = nil
	if err := sqlH.UpdateColumns(&spr, models.SessionPlayerRaceDBAIControlTypeColumn); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// Handle handles the request
func (h *SessionPlayerSwitchToHumanHandler) Handle(
	params operations.SessionPlayerSwitchToHumanParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPlayerSwitchToHumanForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewSessionPlayerSwitchToHumanNotFound().WithPayload(models.FromError(err))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return operations.NewSessionPlayerSwitchToHumanPreconditionFailed().WithPayload(models.FromError(err))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPlayerSwitchToHumanNoContent()
}

// Authorize checks if the user is a session manager or global manager
func (h *SessionPlayerSwitchToHumanHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionPlayerSwitchToHumanParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// Only session managers can switch players to human
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
