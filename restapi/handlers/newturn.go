package handlers

import (
	"net/http"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"context"
	"database/sql"
	"errors"

	"encoding/json"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"orus.io/orus-io/go-orusapi/database"
)

// NewTurnResponder asks for an http.Request because it will upgrade it to a websocket
func NewTurnResponder(req *http.Request, db *sqlx.DB, details *TurnDetails) *TurnResponder {
	return &TurnResponder{details, db, req}
}

// TurnResponder is responsible to upgrade a http.Request to a websocket and then
// to poll the DB until it gets a new turn for the client.
// At that time the new turn will be sent into the websocket as a json payload (text payload)
type TurnResponder struct {
	details *TurnDetails

	db  *sqlx.DB
	req *http.Request
}

func (r *TurnResponder) Logger() *zerolog.Logger {
	return zerolog.Ctx(r.req.Context())
}

var upgrader = websocket.Upgrader{} // Use default options

type NewTurnChan chan *models.TurnFiles

// WriteResponse will upgrade the connection to websocket
func (r *TurnResponder) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
	ctx := r.req.Context()
	logger := zerolog.Ctx(ctx)

	c, err := upgrader.Upgrade(rw, r.req, nil)
	if err != nil {
		logger.Err(err).Msg("turn responder failed to upgrade connection to websocket")
		return
	}
	defer func() {
		if err := c.Close(); err != nil {
			logger.Err(err).Msg("failed to close connection")
		}
	}()

	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()
	newTurnChan := make(NewTurnChan)
	errChan := make(chan error)
	// this should become waitForNewTurn
	// a NATS client waiting on a newturn for this session
	go r.scanForNewTurn(ctx, newTurnChan, errChan)

	pingTicker := time.NewTicker(time.Second * 30)
	defer pingTicker.Stop()

	for {
		select {
		case <-pingTicker.C:
			if err := c.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				// failed to send a ping
				logger.Err(err).Msg("failed to send a ping the ws client")
				return
			}
		case t := <-newTurnChan:
			jsonTurn, err := json.Marshal(t)
			if err != nil {
				logger.Err(err).Msg("failed to marshal newTurn to json")
				return
			}
			if err := c.WriteMessage(websocket.TextMessage, jsonTurn); err != nil {
				logger.Err(err).Msg("failed to send turn into websocket")
				return
			}
		case err := <-errChan:
			logger.Err(err).Msg("failed to obtain a new turn")
			return
		}
	}
}

func (r *TurnResponder) scanForNewTurn(ctx context.Context, newTurnChan NewTurnChan, errChan chan error) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() != nil {
				r.Logger().Err(ctx.Err()).Msg("scan for new turn: context returned an error")
				errChan <- ctx.Err()
			}
			return
		default:
			// only scan the DB once in 30 s
			<-ticker.C
			t, err := r.getNewTurn(ctx)
			if err != nil {
				r.Logger().Err(ctx.Err()).Msg("scan for new turn: failed to query for new turn availability")
				errChan <- err
				return
			}
			newTurnChan <- t
		}
	}
}

func (r *TurnResponder) getNewTurn(ctx context.Context) (*models.TurnFiles, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, r.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var sessionFiles models.SessionFilesDB
	whereClause := sq.And{
		sq.Eq{models.SessionFilesDBSessionIDColumn: r.details.SessionID},
		sq.Eq{models.SessionFilesDBYearColumn: r.details.Year},
	}
	if err := sqlH.GetWhere(&sessionFiles, whereClause); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("session files not found with session ID: " + r.details.SessionID)
		}
		return nil, err
	}
	turnFiles := sessionFiles.ToTurnFiles(r.details.PlayerOrder)
	return &turnFiles, nil
}
