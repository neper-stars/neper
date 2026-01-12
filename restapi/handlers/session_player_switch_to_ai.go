package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/neper-stars/houston/lib/tools/playerchanger"
	"github.com/neper-stars/houston/store"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionPlayerSwitchToAIHandler creates a new handler for switching a player to AI control
func NewSessionPlayerSwitchToAIHandler(log *zerolog.Logger, db *sqlx.DB) *SessionPlayerSwitchToAIHandler {
	return &SessionPlayerSwitchToAIHandler{db: db, log: log}
}

// SessionPlayerSwitchToAIHandler handles POST /sessions/{session_id}/player/{player_order}/switch_to_ai
type SessionPlayerSwitchToAIHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *SessionPlayerSwitchToAIHandler) handle(
	ctx context.Context, params operations.SessionPlayerSwitchToAIParams, principal *models.Principal,
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

	// Query for the latest session files (highest year)
	query := lastSessionFilesQuery(sessionID)
	var sessionFilesDB models.SessionFilesDB
	if err := sqlH.Get(&sessionFilesDB, query); err != nil {
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

	// Parse the AI type
	aiType, err := store.ParseAIExpertType(params.SwitchRequest.AiType)
	if err != nil {
		return errs.NewErrInvalidSomething("invalid AI type: " + err.Error())
	}

	// Check player info before switching - ensure this is not the last human player
	fileInfo, err := playerchanger.ReadPlayersFromBytes("game.hst", hstData)
	if err != nil {
		return errs.NewErrInvalidSomething("failed to read player info: " + err.Error())
	}

	// Count human players (excluding the one we're about to switch)
	humanCount := 0
	for _, player := range fileInfo.Players {
		if player.StatusType == store.PlayerStatusHuman && player.Number != playerOrder {
			humanCount++
		}
	}
	if humanCount == 0 {
		return errs.NewErrPreconditionFailed("cannot switch the last human player to AI")
	}

	// Verify the target player exists and is human
	targetPlayer := fileInfo.GetPlayer(playerOrder)
	if targetPlayer == nil {
		return errs.NewErrPreconditionFailed("player not found in game")
	}
	if targetPlayer.StatusType != store.PlayerStatusHuman {
		return errs.NewErrPreconditionFailed("player is already AI or inactive")
	}

	// Switch the player to AI using houston's playerchanger
	modifiedHst, _, err := playerchanger.ChangeToAIBytes(hstData, playerOrder, aiType)
	if err != nil {
		return errs.NewErrInvalidSomething("failed to switch player to AI: " + err.Error())
	}

	// Encode the modified host file back to base64
	sessionFilesDB.HostFile = stars.B64Encode(modifiedHst)

	// Update the session files in the database
	if err := sqlH.UpdateColumns(&sessionFilesDB, models.SessionFilesDBHostFileColumn); err != nil {
		return err
	}

	// Update session_player_race.ai_control_type to track the control change
	aiTypeStr := params.SwitchRequest.AiType
	if err := h.updateAIControlType(sqlH, sessionID, playerOrder, &aiTypeStr); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// updateAIControlType updates the ai_control_type column in session_player_race
func (h *SessionPlayerSwitchToAIHandler) updateAIControlType(
	sqlH database.SQLHelper, sessionID string, playerOrder int, aiType *string,
) error {
	// Find the session_player_race entry for this player
	var spr models.SessionPlayerRaceDB
	query := sessionPlayerRaceByOrderQuery(sessionID, playerOrder)
	if err := sqlH.Get(&spr, query); err != nil {
		return err
	}
	spr.AIControlType = aiType
	return sqlH.UpdateColumns(&spr, models.SessionPlayerRaceDBAIControlTypeColumn)
}

// Handle handles the request
func (h *SessionPlayerSwitchToAIHandler) Handle(
	params operations.SessionPlayerSwitchToAIParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPlayerSwitchToAIForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewSessionPlayerSwitchToAINotFound().WithPayload(models.FromError(err))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return operations.NewSessionPlayerSwitchToAIPreconditionFailed().WithPayload(models.FromError(err))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPlayerSwitchToAINoContent()
}

// Authorize checks if the user is a session manager or global manager
func (h *SessionPlayerSwitchToAIHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionPlayerSwitchToAIParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// Only session managers can switch players to AI
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
