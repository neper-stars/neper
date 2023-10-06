package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRulesCreateHandler ...
func NewRulesCreateHandler(log *zerolog.Logger, db *sqlx.DB) *RulesCreateHandler {
	return &RulesCreateHandler{db, log}
}

// RulesCreateHandler handles /Races
type RulesCreateHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *RulesCreateHandler) handle(
	ctx context.Context, params operations.RulesCreateParams, principal *models.Principal,
) (*models.Ruleset, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** AUTHORIZATION **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}
	// ** AUTHORIZATION END **

	var rulesetDB models.RulesetDB
	uid, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	rulesetDB.ID = uid.String()
	rulesetDB.SessionID = params.SessionID

	_, err = sqlH.Insert(&rulesetDB)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &rulesetDB.Ruleset, nil
}

// Handle handles the request
func (h *RulesCreateHandler) Handle(
	params operations.RulesCreateParams, principal *models.Principal,
) middleware.Responder {
	ruleset, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewRulesCreateForbidden().WithPayload(&models.Error{
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
	return operations.NewRulesCreateOK().WithPayload(ruleset)
}

func (h *RulesCreateHandler) Authorize(
	sqlH database.SQLHelper, params operations.RulesCreateParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// managers are allowed to set ruleset for the session
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
