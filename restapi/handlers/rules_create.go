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
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRulesCreateHandler ...
func NewRulesCreateHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *RulesCreateHandler {
	return &RulesCreateHandler{db: db, log: log, notifyService: notifyService}
}

// RulesCreateHandler handles /Races
type RulesCreateHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
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

	// Check if the session exists and is not started
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, params.SessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSessionNotFound("session not found with ID: " + params.SessionID)
		}
		return nil, err
	}
	if sessionDB.State != models.SessionStatePending {
		return nil, errs.NewErrSessionAlreadyStarted("cannot modify rules for a session that is not pending")
	}

	// Check if rules already exist for this session
	var existingRuleset models.RulesetDB
	r := models.Schema.RulesetDB.As("r")
	existingQuery := database.SQ.Select().
		Columns(r.FQColumns(true)...).
		From(r.Sql()).
		Where(r.SessionID.Eq(params.SessionID))
	err = sqlH.Get(&existingRuleset, existingQuery)
	rulesExist := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	var rulesetDB models.RulesetDB
	if rulesExist {
		// Update existing ruleset
		rulesetDB.ID = existingRuleset.ID
	} else {
		// Create new ruleset
		uid, err := uuid.V4()
		if err != nil {
			return nil, err
		}
		rulesetDB.ID = uid.String()
	}
	rulesetDB.SessionID = params.SessionID
	rulesetDB.AcceleratedBbsPlay = params.Ruleset.AcceleratedBbsPlay
	rulesetDB.ComputerPlayersFormAlliances = params.Ruleset.ComputerPlayersFormAlliances
	rulesetDB.Density = params.Ruleset.Density
	rulesetDB.GalaxyClumping = params.Ruleset.GalaxyClumping
	rulesetDB.MaximumMinerals = params.Ruleset.MaximumMinerals
	rulesetDB.NoRandomEvents = params.Ruleset.NoRandomEvents
	rulesetDB.PublicPlayerScores = params.Ruleset.PublicPlayerScores
	rulesetDB.SlowerTechAdvances = params.Ruleset.SlowerTechAdvances
	rulesetDB.StartingDistance = params.Ruleset.StartingDistance
	rulesetDB.UniverseSize = params.Ruleset.UniverseSize
	rulesetDB.RandomSeed = params.Ruleset.RandomSeed

	rulesetDB.VcAttainTechXInYField = params.Ruleset.VcAttainTechXInYField
	rulesetDB.VcAttainTechXInYFieldFieldsValue = params.Ruleset.VcAttainTechXInYFieldFieldsValue
	rulesetDB.VcAttainTechXInYFieldTechValue = params.Ruleset.VcAttainTechXInYFieldTechValue

	rulesetDB.VcExceedNextPlayerScoreByx = params.Ruleset.VcExceedNextPlayerScoreByx
	rulesetDB.VcExceedNextPlayerScoreByxValue = params.Ruleset.VcExceedNextPlayerScoreByxValue

	rulesetDB.VcExceedScoreOfx = params.Ruleset.VcExceedScoreOfx
	rulesetDB.VcExceedScoreOfxValue = params.Ruleset.VcExceedScoreOfxValue

	rulesetDB.VcHasProductionCapacityOfxThousand = params.Ruleset.VcHasProductionCapacityOfxThousand
	rulesetDB.VcHasProductionCapacityOfxThousandValue = params.Ruleset.VcHasProductionCapacityOfxThousandValue

	rulesetDB.VcHaveHighestScoreAfterxYears = params.Ruleset.VcHaveHighestScoreAfterxYears
	rulesetDB.VcHaveHighestScoreAfterxYearsValue = params.Ruleset.VcHaveHighestScoreAfterxYearsValue

	rulesetDB.VcOwnsxCapitalShips = params.Ruleset.VcOwnsxCapitalShips

	capitalShipsValue := params.Ruleset.VcOwnsxCapitalShipsValue
	if params.Ruleset.VcOwnsxCapitalShipsValue < 10 {
		capitalShipsValue = 10
	} else if params.Ruleset.VcOwnsxCapitalShipsValue > 300 {
		capitalShipsValue = 300
	}
	rulesetDB.VcOwnsxCapitalShipsValue = capitalShipsValue

	rulesetDB.VcOwnsxPercentOfPlanets = params.Ruleset.VcOwnsxPercentOfPlanets
	rulesetDB.VcOwnsxPercentOfPlanetsValue = params.Ruleset.VcOwnsxPercentOfPlanetsValue

	rulesetDB.VcWinnerMustMeetxOfTheAbove = params.Ruleset.VcWinnerMustMeetxOfTheAbove
	rulesetDB.VcAtLeastxYearsMustPassBeforeaWinnerIsDeclared = params.Ruleset.VcAtLeastxYearsMustPassBeforeaWinnerIsDeclared

	if rulesExist {
		if err := sqlH.Update(&rulesetDB); err != nil {
			return nil, err
		}
	} else {
		if _, err := sqlH.Insert(&rulesetDB); err != nil {
			return nil, err
		}
		// Update session to mark rules as set and set the FK (only needed on first creation)
		sessionDB.RulesIsSet = true
		sessionDB.RulesetID = &rulesetDB.ID
		if err := sqlH.Update(&sessionDB); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Publish notification after successful commit (session was updated with rules)
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionUpdate(params.SessionID)
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
		case errors.Is(err, errs.ErrPreconditionFailed):
			return PreconditionFailed(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
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
