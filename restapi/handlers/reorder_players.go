package handlers

import (
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"

	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/rs/zerolog"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"orus.io/orus-io/go-orusapi/database"
)

// NewReorderPlayersHandler ...
func NewReorderPlayersHandler(log *zerolog.Logger, db *sqlx.DB) *ReorderPlayersHandler {
	return &ReorderPlayersHandler{db, log}
}

// ReorderPlayersHandler handles /circles
type ReorderPlayersHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *ReorderPlayersHandler) handle(
	ctx context.Context, params operations.ReorderPlayersParams, principal *models.Principal,
) ([]*models.PlayerOrder, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}
	players := params.Players

	// sort players by playerOrder
	// create an array of playerID using the resulting list
	// use this array in our sql query

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	for _, player := range players {
		updateBuilder := sq.
			Update(models.SessionPlayerRaceDBTable).
			Set(models.SessionPlayerRaceDBPlayerOrderColumn, player.PlayerOrder).
			Where(sq.Eq{models.SessionPlayerRaceDBUserProfileIDColumn: player.UserProfileID})

		_, err = sqlH.Exec(updateBuilder)
		if err != nil {
			h.log.Err(err).
				Str("playerID", player.UserProfileID).
				Int64("order", player.PlayerOrder).
				Msg("failed to execute reorder for player")
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return params.Players, nil
}

// Handle handles the request
func (h *ReorderPlayersHandler) Handle(
	params operations.ReorderPlayersParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewReorderPlayersForbidden().WithPayload(&models.Error{
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
	return operations.NewReorderPlayersOK().WithPayload(session)
}

func (h *ReorderPlayersHandler) Authorize(
	ctx context.Context, params operations.ReorderPlayersParams, principal *models.Principal,
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
