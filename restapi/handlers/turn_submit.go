package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	hs "github.com/neper-stars/houston"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/sessionSubmitter"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/models/types"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewTurnSubmitHandler ...
func NewTurnSubmitHandler(log *zerolog.Logger, db *sqlx.DB, submitter sessionSubmitter.SessionSubmitter, notifyService *notify.Service) *TurnSubmitHandler {
	configuredLogger := log.With().Str("handler", "TurnSubmitHandler").Logger()
	return &TurnSubmitHandler{db, &configuredLogger, submitter, notifyService}
}

// TurnSubmitHandler handles /session
type TurnSubmitHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	submitter     sessionSubmitter.SessionSubmitter
	notifyService *notify.Service
}

type TurnDetails struct {
	SessionID   string
	Year        int
	PlayerOrder int64
}

func (h *TurnSubmitHandler) handle(
	ctx context.Context, params operations.TurnSubmitParams, principal *models.Principal,
) (*TurnDetails, error) {
	log := *zerolog.Ctx(ctx)
	log = log.With().
		Str("handler", "TurnSubmitHandler").
		Str("sessionID", params.SessionID).
		Str("userNickName", principal.NickName).
		Logger()

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

	var userSessionSetup models.SessionPlayerRaceDB
	query := sessionPlayerRaceQuery(principal.Subject, params.SessionID)
	if err := sqlH.Get(&userSessionSetup, query); err != nil {
		h.log.Err(err).Msg("failed to fetch user profile session relation")
		return nil, err
	}

	// perform some verifications on the submitted file
	data, err := stars.B64Decode(params.Turnfile.B64Data)
	if err != nil {
		h.log.Err(err).Msg("failed to parse submitted order file")
		return nil, errs.NewErrInvalidSomething("invalid turn file: " + err.Error())
	}
	playerOrder := userSessionSetup.PlayerOrder

	fd := hs.FileData(data)
	header, err := fd.FileHeader()
	if err != nil {
		return nil, err
	}
	if int64(header.PlayerIndex()) != playerOrder {
		h.log.Warn().
			Int64("playerOrder", playerOrder).
			Int("playerIndex", header.PlayerIndex()).
			Msg("player index in the submitted order file does not match player order in session")
		return nil, errs.NewErrInvalidSomething(fmt.Sprintf("invalid player index in order file, is: %d, should be: %d", header.PlayerIndex(), userSessionSetup.PlayerOrder))
	}

	if !header.TurnSubmitted() {
		h.log.Warn().Msg("received a non-submitted order file... will not store this")
		return nil, errs.NewErrInvalidSomething("you should save and submit your turn in stars! before sending it")
	}

	query = lastSessionFilesQuery(params.SessionID)
	var sessionFilesDB models.SessionFilesDB
	if err := sqlH.Get(&sessionFilesDB, query); err != nil {
		return nil, err
	}

	if sessionFilesDB.Year != int64(header.Year()) {
		return nil, errs.NewErrInvalidSomething(fmt.Sprintf("turn year is not matching our awaiting year, got %d, wanted %d", header.Year(), sessionFilesDB.Year))
	}

	numPlayers := len(sessionFilesDB.Turns)
	if len(sessionFilesDB.Orders) == 0 {
		// initialize the orders
		for i := 0; i < numPlayers; i++ {
			if int64(i) == playerOrder {
				sessionFilesDB.Orders = append(sessionFilesDB.Orders, types.Order{B64Data: params.Turnfile.B64Data})
			} else {
				sessionFilesDB.Orders = append(sessionFilesDB.Orders, types.Order{B64Data: ""})
			}
		}
	} else {
		// update the player order or die
		if sessionFilesDB.Orders[playerOrder].B64Data == "" {
			sessionFilesDB.Orders[playerOrder] = types.Order{B64Data: params.Turnfile.B64Data}
		} else {
			return nil, errs.NewErrInvalidSomething("cannot submit twice your orders")
		}
	}

	// then store the submitted order file in the database, after verifying it is not already present
	if err := sqlH.UpdateColumns(&sessionFilesDB, models.SessionFilesDBOrdersColumn); err != nil {
		return nil, err
	}

	// verify if all players are ready
	readyPlayers := sessionFilesDB.ReadyPlayers()

	var sessionPlayerRaces []models.SessionPlayerRaceDB
	query = sessionPlayerRaceForSessionQuery(params.SessionID)
	if err := sqlH.Select(&sessionPlayerRaces, query); err != nil {
		h.log.Err(err).Msg("failed to fetch session player races for session")
		return nil, err
	}

	var allPlayersReady = true
	if len(readyPlayers) == 0 {
		allPlayersReady = false
	} else {
		for i, race := range sessionPlayerRaces {
			if race.IsBot {
				// bot are never "ready", the game engine
				// plays their turn when all other players are ready,
				// and we ask it to generate a turn
				continue
			}
			if !readyPlayers[i] {
				allPlayersReady = false
				break
			}
		}
	}

	if err := tx.Commit(); err != nil {
		h.log.Err(err).Str("sessionID", params.SessionID).Msg("failed save turn into DB")
		return nil, err
	}

	// Publish order status update notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishOrderStatusUpdate(params.SessionID, sessionFilesDB.Year)
	}

	if allPlayersReady {
		// here we want to submit our database transaction to allow the turn generator to find our updated session

		// then try to warn turn generator it has job to do
		if err := h.submitter.Sub(params.SessionID, params.Year); err != nil {
			// here we failed to publish our message.
			// This should not be a big issue because in the long run the
			// turn generator should auto find sessions that need generation
			// let's just log this error
			h.log.Err(err).Str("sessionID", params.SessionID).Msg("failed to signal the turn generator, our session is ready")
		}
	}

	return &TurnDetails{
		SessionID:   params.SessionID,
		Year:        header.Year(),
		PlayerOrder: playerOrder,
	}, nil
}

// Handle handles the request
func (h *TurnSubmitHandler) Handle(
	params operations.TurnSubmitParams, principal *models.Principal,
) middleware.Responder {
	_, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewTurnSubmitForbidden().WithPayload(&models.Error{
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
	return operations.NewTurnSubmitOK()
}

func (h *TurnSubmitHandler) Authorize(
	sqlH database.SQLHelper, params operations.TurnSubmitParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	var authRes models.UserProfileSessionRelDB
	query := userProfileSessionRelationQuery(principal.Subject, params.SessionID)
	if err := sqlH.Get(&authRes, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.log.Debug().Msg("auth: no matching userprofile session relation --> reject")
			return false, nil
		}
		return false, err
	}
	return true, nil
}
