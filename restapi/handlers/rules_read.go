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

// NewRulesReadHandler ...
func NewRulesReadHandler(log *zerolog.Logger, db *sqlx.DB) *RulesReadHandler {
	return &RulesReadHandler{db, log}
}

// RulesReadHandler handles /Races
type RulesReadHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *RulesReadHandler) handle(
	ctx context.Context, params operations.RulesReadParams, principal *models.Principal,
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

	sessionID := params.SessionID

	var rulesetDB models.RulesetDB
	if err := sqlH.GetBy(&rulesetDB, models.RulesetDBSessionIDColumn, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrRulesNotFound("rules not found for session ID: " + sessionID)
		}
		return nil, err
	}
	return &rulesetDB.Ruleset, nil
}

// Handle handles the request
func (h *RulesReadHandler) Handle(
	params operations.RulesReadParams, principal *models.Principal,
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

func (h *RulesReadHandler) Authorize(
	sqlH database.SQLHelper, params operations.RulesReadParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// members are allowed to get ruleset for the session
	return IsSessionMember(sqlH, principal.Subject, params.SessionID)
}
