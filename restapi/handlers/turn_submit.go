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

	"strings"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewTurnSubmitHandler ...
func NewTurnSubmitHandler(log *zerolog.Logger, db *sqlx.DB) *TurnSubmitHandler {
	configuredLogger := log.With().Str("handler", "TurnSubmitHandler").Logger()
	return &TurnSubmitHandler{db, &configuredLogger}
}

// TurnSubmitHandler handles /session
type TurnSubmitHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
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
				sessionFilesDB.OrdersDB = append(sessionFilesDB.OrdersDB, models.Order{B64Data: params.Turnfile.B64Data})
			} else {
				sessionFilesDB.OrdersDB = append(sessionFilesDB.OrdersDB, models.Order{B64Data: ""})
			}
		}
	} else {
		// update the player order or die
		if sessionFilesDB.OrdersDB[playerOrder].B64Data == "" {
			sessionFilesDB.Orders[playerOrder] = &models.Order{B64Data: params.Turnfile.B64Data}
		} else {
			return nil, errs.NewErrInvalidSomething("cannot submit twice your orders")
		}
	}

	// then store the submitted order file in the database, after verifying it is not already present
	if err := sqlH.UpdateColumns(&sessionFilesDB, models.SessionFilesDBOrdersDBColumn); err != nil {
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
	}
	for i, race := range sessionPlayerRaces {
		if race.IsBot {
			// bot are never "ready", the game engine
			// plays their turn when all other players are ready
			// and we ask it to generate a turn
			continue
		}
		if !readyPlayers[i] {
			allPlayersReady = false
			break
		}
	}
	if allPlayersReady {
		// TODO, we will mark the session as to be generated, and a goroutine dedicated to generating
		// turns will do its work and update the sessionFiles with a new year
		// the client will be able to subscribe to a websocket and ask to be pushed his new turn
		// when the next year becomes available
		h.log.Debug().Msg("we will launch the turn generation because all players are ready")
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
	result, err := h.handle(params.HTTPRequest.Context(), params, principal)
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
	if wantsWebSocket(params.HTTPRequest) {
		// if the client wants a websocket upgrade let's give it a turn responder
		// this will open a websocket that will push a new turn when it becomes available
		return NewTurnResponder(params.HTTPRequest, h.db, result)
	}
	return operations.NewTurnSubmitOK()
}

func wantsWebSocket(r *http.Request) bool {
	var connection string
	var upgrade string

	for k, v := range r.Header {
		if strings.ToLower(k) == "connection" { // Connection header
			connection = strings.Join(v, "")
		} else if strings.ToLower(k) == "upgrade" { // Upgrade header
			upgrade = strings.Join(v, "")
		}
	}
	if strings.ToLower(connection) == "upgrade" && upgrade == "websocket" {
		return true
	}
	return false
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
