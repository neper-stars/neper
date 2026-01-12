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

// NewSessionPlayerControlHandler creates a new handler for getting player control status
func NewSessionPlayerControlHandler(log *zerolog.Logger, db *sqlx.DB) *SessionPlayerControlHandler {
	return &SessionPlayerControlHandler{db: db, log: log}
}

// SessionPlayerControlHandler handles GET /sessions/{session_id}/player_control
type SessionPlayerControlHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

// playerControlInfo is used for the JOIN query result
type playerControlInfo struct {
	PlayerOrder   int64   `db:"player_order"`
	UserProfileID string  `db:"user_profile_id"`
	Nickname      string  `db:"nickname"`
	IsBot         bool    `db:"is_bot"`
	AIControlType *string `db:"ai_control_type"`
}

func (h *SessionPlayerControlHandler) handle(
	ctx context.Context, params operations.SessionPlayerControlParams, principal *models.Principal,
) (*models.PlayerControlList, error) {
	sessionID := params.SessionID

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** Authorization - managers only **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	// Verify session exists
	var session models.SessionDB
	if err := sqlH.GetByPKey(&session, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSessionNotFound("session not found with ID: " + sessionID)
		}
		return nil, err
	}

	// Query session_player_race with user_profile join to get nicknames
	spr := models.Schema.SessionPlayerRaceDB.As("spr")
	up := models.Schema.UserProfileDB.As("up")

	q := database.SQ.Select().
		Column(spr.PlayerOrder.Sql()).
		Column(spr.UserProfileID.Sql()).
		Column(up.Nickname.Sql() + " AS nickname").
		Column(spr.IsBot.Sql()).
		Column(spr.AIControlType.Sql()).
		From(spr.Sql()).
		LeftJoin(spr.UserProfileID.Join(up.ID).Sql()).
		Where(spr.SessionID.Eq(sessionID)).
		OrderBy(spr.PlayerOrder.Sql())

	var list []playerControlInfo
	if err := sqlH.Select(&list, q); err != nil {
		return nil, err
	}

	// Convert to response format
	players := make([]*models.PlayerControlStatus, len(list))
	for i, p := range list {
		controlStatus := models.PlayerControlStatusControlStatusHuman
		if p.AIControlType != nil {
			controlStatus = models.PlayerControlStatusControlStatusAi
		}
		players[i] = &models.PlayerControlStatus{
			PlayerOrder:   p.PlayerOrder,
			UserProfileID: p.UserProfileID,
			Nickname:      p.Nickname,
			IsBot:         p.IsBot,
			AiControlType: p.AIControlType,
			ControlStatus: controlStatus,
		}
	}

	return &models.PlayerControlList{Players: players}, nil
}

// Handle handles the request
func (h *SessionPlayerControlHandler) Handle(
	params operations.SessionPlayerControlParams, principal *models.Principal,
) middleware.Responder {
	result, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPlayerControlForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewSessionPlayerControlNotFound().WithPayload(models.FromError(err))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPlayerControlOK().WithPayload(result)
}

// Authorize checks if the user is a session manager or global manager
func (h *SessionPlayerControlHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionPlayerControlParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// Only session managers can view player control status
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
