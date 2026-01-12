package handlers

import (
	"context"
	"database/sql"
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

// NewOrdersStatusHandler creates a new handler for getting order submission status
func NewOrdersStatusHandler(log *zerolog.Logger, db *sqlx.DB) *OrdersStatusHandler {
	configuredLogger := log.With().Str("handler", "OrdersStatusHandler").Logger()
	return &OrdersStatusHandler{db: db, log: &configuredLogger}
}

// OrdersStatusHandler handles GET /sessions/{session_id}/orders/{year}
type OrdersStatusHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

// playerWithOrder holds the joined data for a player and their order submission status
type playerWithOrder struct {
	PlayerOrder   int64   `db:"player_order"`
	UserProfileID string  `db:"user_profile_id"`
	Nickname      string  `db:"nickname"`
	BotLevel      *int64  `db:"bot_level"`
	AIControlType *string `db:"ai_control_type"`
}

func (h *OrdersStatusHandler) handle(
	ctx context.Context, params operations.OrdersStatusParams, principal *models.Principal,
) ([]*models.PlayerOrderStatus, error) {
	sessionID := params.SessionID
	year := params.Year

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** Authorization **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	// Get session files for this year to check which orders have been submitted
	var sessionFiles models.SessionFilesDB
	sfQuery := database.SQ.Select().
		Columns(models.SessionFilesDBColumns...).
		From(models.SessionFilesDBTable).
		Where(sq.And{
			sq.Eq{models.SessionFilesDBSessionIDColumn: sessionID},
			sq.Eq{models.SessionFilesDBYearColumn: year},
		})

	if err := sqlH.Get(&sessionFiles, sfQuery); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("no turn files found for session and year")
		}
		return nil, err
	}

	// Get the list of players with their order and nicknames
	spr := models.Schema.SessionPlayerRaceDB.As("spr")
	u := models.Schema.UserProfileDB.As("u")

	query := database.SQ.Select().
		Column(spr.PlayerOrder.Sql()).
		Column(spr.UserProfileID.Sql()).
		Column(spr.BotLevel.Sql()).
		Column(spr.AIControlType.Sql()).
		Column("COALESCE(" + u.Nickname.Sql() + ", 'Bot') AS nickname").
		From(spr.Sql()).
		LeftJoin(spr.UserProfileID.Join(u.ID).Sql()).
		Where(sq.Eq{spr.SessionID.Sql(): sessionID}).
		OrderBy(spr.PlayerOrder.Sql() + " ASC")

	var players []playerWithOrder
	if err := sqlH.Select(&players, query); err != nil {
		return nil, err
	}

	// Build the result
	readyPlayers := sessionFiles.ReadyPlayers()
	result := make([]*models.PlayerOrderStatus, 0, len(players))

	for _, player := range players {
		isBot := player.BotLevel != nil
		submitted := false
		if int(player.PlayerOrder) < len(readyPlayers) {
			submitted = readyPlayers[player.PlayerOrder]
		}

		result = append(result, &models.PlayerOrderStatus{
			PlayerOrder:   player.PlayerOrder,
			Nickname:      player.Nickname,
			IsBot:         isBot,
			AiControlType: player.AIControlType,
			Submitted:     submitted,
		})
	}

	return result, nil
}

// Handle handles the request
func (h *OrdersStatusHandler) Handle(
	params operations.OrdersStatusParams, principal *models.Principal,
) middleware.Responder {
	result, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewOrdersStatusForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewOrdersStatusNotFound().WithPayload(models.FromError(err))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewOrdersStatusOK().WithPayload(result)
}

func (h *OrdersStatusHandler) Authorize(
	sqlH database.SQLHelper, params operations.OrdersStatusParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// Only session members can access order status
	return IsSessionMember(sqlH, principal.Subject, params.SessionID)
}
