package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	jsoniter "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/zenazn/goji/web/mutil"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
)

// NewTurnResponder asks for a http.Request because it will upgrade it to a websocket
func NewTurnResponder(req *http.Request, db *sqlx.DB, details *TurnDetails, natsConn *nats.Conn) (*TurnResponder, error) {
	newTurnChan := make(chan *nats.Msg)
	sub, err := natsConn.ChanSubscribe(stars.SubjectTurnNotifyForSession(details.SessionID), newTurnChan)
	if err != nil {
		return nil, err
	}
	ctx := req.Context()
	logger := zerolog.Ctx(ctx)
	tr := &TurnResponder{
		details:     details,
		db:          db,
		req:         req,
		natsSub:     sub,
		newTurnChan: newTurnChan,
		logger:      logger,
	}
	return tr, nil
}

// TurnResponder is responsible to upgrade a http.Request to a websocket and then
// to poll the DB until it gets a new turn for the client.
// At that time the new turn will be sent into the websocket as a json payload (text payload)
type TurnResponder struct {
	details *TurnDetails

	db          *sqlx.DB
	req         *http.Request
	natsSub     *nats.Subscription
	newTurnChan chan *nats.Msg
	logger      *zerolog.Logger
}

func (r *TurnResponder) Shutdown() {
	if err := r.natsSub.Unsubscribe(); err != nil {
		r.logger.Err(err).Msg("failed to unsubscribe from nats subject")
	}
}

func (r *TurnResponder) Logger() *zerolog.Logger {
	return zerolog.Ctx(r.req.Context())
}

var upgrader = websocket.Upgrader{} // Use default options

type NewTurnChan chan *models.TurnFiles

// WriteResponse will upgrade the connection to websocket
func (r *TurnResponder) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
	ctx := r.req.Context()

	// t := reflect.TypeOf(rw).String()
	// r.logger.Info().Str("rw type", t).Msg("")
	// here we know that the response writer is wrapped by zerolog,
	// so we type assert on the interface used by zerolog
	rw_ := rw.(mutil.WriterProxy)
	// then we use the Unwrap method to get to the original ResponseWriter
	// this one implements Hijacker
	// once this is done we can pass it down the Upgrade method that requires the Hijacker interface
	c, err := upgrader.Upgrade(rw_.Unwrap(), r.req, nil)
	if err != nil {
		r.logger.Err(err).Msg("turn responder failed to upgrade connection to websocket")
		return
	}
	defer func() {
		if err := c.Close(); err != nil {
			r.logger.Err(err).Msg("failed to close connection")
		}
		// unsubscribe
		r.Shutdown()
	}()

	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()
	newSQLTurnChan := make(NewTurnChan)
	errChan := make(chan error)
	// This scan indefinitely on the SQL database to find the new turn from time to time
	go r.scanForNewTurn(ctx, newSQLTurnChan, errChan)

	pingTicker := time.NewTicker(time.Second * 30)
	defer pingTicker.Stop()

	for {
		select {
		case <-pingTicker.C:
			if err := c.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				// failed to send a ping
				r.logger.Err(err).Msg("failed to send a ping the ws client")
				return
			}
		case msg := <-r.newTurnChan:
			// we got an explicit message from the turn generator that our turn is ready
			// let's use this message data
			var sf *models.SessionFiles
			if err := jsoniter.Unmarshal(msg.Data, sf); err != nil {
				r.logger.Err(err).Msg("failed to unmarshal sessionFile from nats message: someone sent an invalid message into this subject ?")
				return
			}
			sfdb := &models.SessionFilesDB{SessionFiles: *sf}
			t := sfdb.ToTurnFiles(r.details.PlayerOrder)

			jsonTurn, err := json.Marshal(t)
			if err != nil {
				r.logger.Err(err).Msg("failed to marshal newTurn to json")
				return
			}
			if err := c.WriteMessage(websocket.TextMessage, jsonTurn); err != nil {
				r.logger.Err(err).Msg("failed to send turn into websocket")
				return
			}
			// we finished our job
			return

		case t := <-newSQLTurnChan:
			// we cannot wait eternally on our turn generator because something may have
			// happened, like a reboot and our message could have long been lost
			// and the turn could be ready in the database
			// so from time to time a goroutine awakes and scans the db to find a turn
			// if it finds something it will send it in this newSQLTurnChan
			jsonTurn, err := json.Marshal(t)
			if err != nil {
				r.logger.Err(err).Msg("failed to marshal newTurn to json")
				return
			}
			if err := c.WriteMessage(websocket.TextMessage, jsonTurn); err != nil {
				r.logger.Err(err).Msg("failed to send turn into websocket")
				return
			}
			// we finished the job
			return
		case err := <-errChan:
			r.logger.Err(err).Msg("failed to obtain a new turn")
			return
		}
	}
}

func (r *TurnResponder) scanForNewTurn(ctx context.Context, newSQLTurnChan NewTurnChan, errChan chan error) {
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
		case <-ticker.C:
			// only scan the DB once in 30 s
			t, err := r.getNewTurn(ctx)
			// only log errors for something else than we did not find your turn (yet)
			if err != nil && !errors.Is(err, errs.ErrNotFound) {
				r.Logger().Err(ctx.Err()).Msg("scan for new turn: failed to query for new turn availability")
				errChan <- err
				return
			}
			newSQLTurnChan <- t
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
