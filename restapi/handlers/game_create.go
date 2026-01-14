package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/neper-stars/houston/store"
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

	// Phase 1: Validate and set state to "starting" to prevent concurrent start commands
	var sessionDB models.SessionDB
	var sessionPlayerRaces []models.SessionPlayerRace
	var ruleset *models.Ruleset
	var races []models.Race

	if err := func() error {
		tx, err := database.Begin(ctx, h.db)
		if err != nil {
			return err
		}
		defer tx.RollbackIfOpened(log)
		sqlH := database.NewSQLHelper(ctx, tx, log)

		// Lock the session row to prevent concurrent start commands
		// FOR UPDATE ensures only one transaction can proceed at a time
		if err := sqlH.GetByPKeyForUpdate(&sessionDB, sessionID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errs.NewErrSessionNotFound("session not found: " + sessionID)
			}
			h.log.Err(err).Msg("failed to get session from DB")
			return err
		}

		// Check session state - only pending sessions can be started
		switch sessionDB.State {
		case models.SessionStateStarting:
			return errs.NewErrInvalidSomething("game generation is already in progress")
		case models.SessionStateStarted:
			return errs.NewErrInvalidSomething("session is already started")
		case models.SessionStateArchived:
			return errs.NewErrInvalidSomething("cannot start an archived session")
		case models.SessionStatePending:
			// OK to proceed
		default:
			return errs.NewErrInvalidSomething("invalid session state: " + sessionDB.State)
		}

		sessionPlayerRaces, err = sessionDB.SessionPlayerRaces(&sqlH)
		if err != nil {
			h.log.Err(err).Str("sessionID", sessionID).Msg("failed to get player races for session")
			return err
		}

		// Normalize player orders to be contiguous (0, 1, 2, ...) without gaps.
		// Gaps can occur when a player leaves before the game starts.
		// This is important because the Turns array is indexed by player order.
		sessionPlayerRaces, err = NormalizePlayerOrders(&sqlH, sessionID, sessionPlayerRaces)
		if err != nil {
			h.log.Err(err).Str("sessionID", sessionID).Msg("failed to normalize player orders")
			return err
		}

		// Check that all players are ready
		for _, spr := range sessionPlayerRaces {
			if !spr.Ready {
				return errs.NewErrPlayersNotReady("all players must be ready before starting the game")
			}
		}

		ruleset, err = sessionDB.Ruleset(&sqlH)
		if err != nil {
			h.log.Err(err).Str("sessionID", sessionID).Msg("failed to get ruleset for session")
			return err
		}

		races, err = sessionDB.PlayerRaces(&sqlH, sessionPlayerRaces)
		if err != nil {
			h.log.Err(err).Str("sessionID", sessionID).Msg("failed to get player races for session")
			return err
		}

		// Validate all races before starting the game to ensure Stars! binary won't fail silently
		var allValidationErrs []string
		for _, race := range races {
			rawData, err := race.RawData()
			if err != nil {
				h.log.Err(err).Str("raceID", race.ID).Msg("failed to decode race data for validation")
				allValidationErrs = append(allValidationErrs, fmt.Sprintf("race %s: failed to decode data", race.NamePlural))
				continue
			}
			_, validationErrs := store.ValidateRaceData(rawData)
			if len(validationErrs) > 0 {
				for _, ve := range validationErrs {
					allValidationErrs = append(allValidationErrs, fmt.Sprintf("race %s: %s", race.NamePlural, ve.Error()))
				}
			}
		}
		if len(allValidationErrs) > 0 {
			h.log.Warn().Strs("validation_errors", allValidationErrs).Str("sessionID", sessionID).Msg("race validation failed before game start")
			return errs.NewErrInvalidRace("race validation failed: " + strings.Join(allValidationErrs, "; "))
		}

		// Set state to "starting" to prevent concurrent start commands
		sessionDB.State = models.SessionStateStarting
		if err := sqlH.Update(&sessionDB); err != nil {
			h.log.Err(err).Msg("failed to update session to starting state")
			return err
		}

		return tx.Commit()
	}(); err != nil {
		return nil, err
	}

	// Phase 2: Generate the game (this can take time)
	// If this fails, we need to revert the state to "pending"
	gameInput := stars.NewGameInput(h.log, sessionID, sessionDB.Name, *ruleset, sessionPlayerRaces)
	gameFiles, err := h.runner.NewGame(ctx, h.log, sessionID, gameInput, sessionPlayerRaces, races)
	if err != nil {
		h.log.Err(err).Msg("failed to create new game")
		// Revert state to pending
		h.revertSessionState(ctx, sessionID, models.SessionStatePending)
		return nil, err
	}

	var sfDB models.SessionFilesDB
	if err := gameFiles.HydrateSessionFiles(&sfDB.SessionFiles); err != nil {
		h.log.Err(err).Msg("failed to parse game files")
		h.revertSessionState(ctx, sessionID, models.SessionStatePending)
		return nil, err
	}

	// Phase 3: Save results and set state to "started"
	if err := func() error {
		tx, err := database.Begin(ctx, h.db)
		if err != nil {
			return err
		}
		defer tx.RollbackIfOpened(log)
		sqlH := database.NewSQLHelper(ctx, tx, log)

		id, err := uuid.V4()
		if err != nil {
			return err
		}
		sfDB.ID = id.String()
		sfDB.SessionID = sessionID

		if _, err := sqlH.Insert(&sfDB); err != nil {
			h.log.Err(err).Msg("failed to insert session files in database")
			return err
		}

		// Mark the session as started
		sessionDB.State = models.SessionStateStarted
		if err := sqlH.Update(&sessionDB); err != nil {
			h.log.Err(err).Msg("failed to update session started flag")
			return err
		}

		return tx.Commit()
	}(); err != nil {
		h.revertSessionState(ctx, sessionID, models.SessionStatePending)
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

// revertSessionState reverts the session state in case of failure during game generation
func (h *GameCreateHandler) revertSessionState(ctx context.Context, sessionID string, state string) {
	log := h.log.With().Str("sessionID", sessionID).Logger()
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		log.Err(err).Msg("failed to begin transaction to revert session state")
		return
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		log.Err(err).Msg("failed to get session to revert state")
		return
	}

	sessionDB.State = state
	if err := sqlH.Update(&sessionDB); err != nil {
		log.Err(err).Msg("failed to revert session state")
		return
	}

	if err := tx.Commit(); err != nil {
		log.Err(err).Msg("failed to commit session state revert")
	}
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
	// Pending users cannot create games
	if principal.IsPending {
		return false, nil
	}
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
