package handlers

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

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
	ctx context.Context, principal *models.Principal,
) ([]*models.Race, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sql := database.NewSQLHelper(ctx, tx, log)

	var list []*models.RaceDB

	q := sq.Select(models.RaceColumns...).
		From(models.RaceDBTable).
		OrderBy(models.RaceDBNameSingularColumn)

	// global manager sees all races, other users only see their own
	if !principal.IsGlobalManager {
		// only get races for the current user
		q = q.Where(sq.Eq{models.RaceDBUserIDColumn: principal.Subject})
	}

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
	sessions, err := h.handle(params.HTTPRequest.Context(), principal)
	if err != nil {
		return operations.NewRacesListDefault(500).WithPayload(models.FromError(err))
	}
	return operations.NewRacesListOK().WithPayload(sessions)
}
