package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewGameCreateHandler ...
func NewGameCreateHandler(log *zerolog.Logger, db *sqlx.DB, runner *stars.Runner, notifyService *notify.Service) *GameCreateHandler {
	return &GameCreateHandler{log: log, db: db, runner: runner, notifyService: notifyService}
}

// GameCreateHandler handles /sessions
type GameCreateHandler struct {
	log           *zerolog.Logger
	db            *sqlx.DB
	runner        *stars.Runner
	notifyService *notify.Service
}

func (h *GameCreateHandler) handle(
	ctx context.Context, params operations.GameCreateParams, principal *models.Principal,
) (*models.TurnFiles, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	sessionID := params.SessionID

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSessionNotFound("session not found: " + sessionID)
		}
		h.log.Err(err).Msg("failed to get session from DB")
		return nil, err
	}

	sessionPlayerRaces, err := sessionDB.SessionPlayerRaces(&sqlH)
	if err != nil {
		h.log.Err(err).Str("sessionID", sessionID).Msg("failed to get player races for session")
		return nil, err
	}

	// Check that all players are ready
	for _, spr := range sessionPlayerRaces {
		if !spr.Ready {
			return nil, errs.NewErrPlayersNotReady("all players must be ready before starting the game")
		}
	}

	ruleset, err := sessionDB.Ruleset(&sqlH)
	if err != nil {
		h.log.Err(err).Str("sessionID", sessionID).Msg("failed to get ruleset for session")
		return nil, err
	}

	races, err := sessionDB.PlayerRaces(&sqlH, sessionPlayerRaces)
	if err != nil {
		h.log.Err(err).Str("sessionID", sessionID).Msg("failed to get player races for session")
		return nil, err
	}

	gameInput := stars.NewGameInput(h.log, sessionID, sessionDB.Name, *ruleset, sessionPlayerRaces)
	gameFiles, err := h.runner.NewGame(ctx, h.log, sessionID, gameInput, sessionPlayerRaces, races)
	if err != nil {
		h.log.Err(err).Msg("failed to create new game")
		return nil, err
	}

	var sfDB models.SessionFilesDB
	if err := gameFiles.HydrateSessionFiles(&sfDB.SessionFiles); err != nil {
		h.log.Err(err).Msg("failed to parse game files")
		return nil, err
	}

	id, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	sfDB.ID = id.String()
	sfDB.SessionID = sessionID

	if _, err := sqlH.Insert(&sfDB); err != nil {
		h.log.Err(err).Msg("failed to insert session files in database")
		return nil, err
	}

	// Mark the session as started
	sessionDB.Started = true
	if err := sqlH.Update(&sessionDB); err != nil {
		h.log.Err(err).Msg("failed to update session started flag")
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Cleanup session directory after successful DB commit
	if err := h.runner.CleanupSessionDir(sessionID); err != nil {
		// Log but don't fail - files are safely in DB
		h.log.Warn().Err(err).Str("sessionID", sessionID).Msg("failed to cleanup session directory after game creation")
	}

	// Publish notifications after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionUpdate(sessionID)
		_ = h.notifyService.PublishSessionTurnReady(sessionID, sfDB.Year)
	}

	// Find the requesting user's player order to return their turn file
	var playerOrder int64
	for _, spr := range sessionPlayerRaces {
		if spr.UserProfileID == principal.Subject {
			playerOrder = spr.PlayerOrder
			break
		}
	}

	turnFiles := sfDB.ToTurnFiles(playerOrder)
	return &turnFiles, nil
}

// Handle handles the request
func (h *GameCreateHandler) Handle(
	params operations.GameCreateParams, principal *models.Principal,
) middleware.Responder {
	turnFiles, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewGameCreateForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return PreconditionFailed(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewGameCreateCreated().WithPayload(turnFiles)
}

func (h *GameCreateHandler) Authorize(
	ctx context.Context, params operations.GameCreateParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	var authRes models.UserProfileSessionRelDB
	query := userProfileSessionRelationQuery(principal.Subject, params.SessionID)
	if err := database.GetContext(ctx, h.db, &authRes, query, h.log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.log.Debug().Msg("auth: no matching userprofile session relation --> reject")
			return false, nil
		}
		return false, err
	}
	return authRes.IsManager, nil
}
