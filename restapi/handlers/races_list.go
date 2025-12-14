package handlers

import (
	"context"
	"errors"
	"net/http"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRacesListHandler ...
func NewRacesListHandler(db *sqlx.DB) *RacesListHandler {
	return &RacesListHandler{db}
}

// RacesListHandler handles /races
type RacesListHandler struct {
	db *sqlx.DB
}

func (h *RacesListHandler) handle(
	ctx context.Context, params operations.RacesListParams, principal *models.Principal,
) ([]*models.Race, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sql := database.NewSQLHelper(ctx, tx, log)

	var list []*models.RaceDB

	q := database.SQ.Select(models.RaceColumns...).
		From(models.RaceDBTable).
		OrderBy(models.RaceDBNameSingularColumn)

	// only get races for the asked user (authorize made sure we cant read others)
	q = q.Where(sq.Eq{models.RaceDBUserIDColumn: params.UserProfileID})

	if err := sql.Select(&list, q); err != nil {
		return nil, err
	}

	var retList []*models.Race
	for i := range list {
		retList = append(retList, &list[i].Race)
	}
	return retList, nil
}

// Handle handles the request
func (h *RacesListHandler) Handle(
	params operations.RacesListParams, principal *models.Principal,
) middleware.Responder {
	sessions, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewRacesListForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		default:
			return operations.NewRacesListDefault(500).WithPayload(models.FromError(err))
		}
	}
	return operations.NewRacesListOK().WithPayload(sessions)
}

func (h *RacesListHandler) Authorize(
	ctx context.Context, params operations.RacesListParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	if params.UserProfileID == principal.Subject {
		return true, nil
	}
	return false, nil
}
