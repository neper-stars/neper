package stars

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-cmd/cmd"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
)

const (
	SubjectTurnNeedsGeneration = "Turn.NeedsGeneration"
)

// TurnGenerator listens on NATs for Turn.NeedsGeneration subjects
// when received it will get the files out of the db, save them in
// a directory and launch stars!.exe on the files to generate the new
// turn.
// Get back the new turn files and save them into database.
// Once done it will submit a notification to the clients using
// the notifyService.
type TurnGenerator struct {
	log           *zerolog.Logger
	runner        *Runner
	natsConn      *nats.Conn
	db            *sqlx.DB
	notifyService *notify.Service

	running    bool
	cancelFunc context.CancelFunc
	ready      chan struct{} // closed when Run() completes initialization (success or failure)
	readyOnce  sync.Once     // ensures ready channel is closed exactly once
	done       chan struct{} // closed when Run() goroutine exits
	doneOnce   sync.Once     // ensures done channel is closed exactly once
}

// NewTurnGenerator is the constructor for the TurnGenerator
func NewTurnGenerator(log *zerolog.Logger, natsConn *nats.Conn, db *sqlx.DB, runner *Runner, notifyService *notify.Service) *TurnGenerator {
	return &TurnGenerator{
		log:           log,
		runner:        runner,
		natsConn:      natsConn,
		db:            db,
		notifyService: notifyService,
		ready:         make(chan struct{}),
		done:          make(chan struct{}),
	}
}

// Ready returns a channel that is closed when the TurnGenerator has completed initialization.
// The channel is closed regardless of whether initialization succeeded or failed.
// Check IsRunning() after Ready() returns to determine if startup was successful.
func (g *TurnGenerator) Ready() <-chan struct{} {
	return g.ready
}

// IsRunning returns true if the TurnGenerator is running successfully.
func (g *TurnGenerator) IsRunning() bool {
	return g.running
}

// signalReady closes the ready channel exactly once.
func (g *TurnGenerator) signalReady() {
	g.readyOnce.Do(func() {
		close(g.ready)
	})
}

// signalDone closes the done channel exactly once.
func (g *TurnGenerator) signalDone() {
	g.doneOnce.Do(func() {
		close(g.done)
	})
}

// Done returns a channel that is closed when the Run() goroutine has fully exited.
func (g *TurnGenerator) Done() <-chan struct{} {
	return g.done
}

// Run is intended to be executed in a goroutine
// when you want to shut down the TurnGenerator.Run goroutine
// just cancel the context you passed in the Run command
func (g *TurnGenerator) Run(ctx context.Context) {
	defer g.signalDone() // signal that we have fully exited

	ctx, fn := context.WithCancel(ctx)
	g.cancelFunc = fn
	needsGenerationChan := make(chan *nats.Msg)

	// we want to listen on the SubjectTurnNeedsGeneration and SubjectTurnWant subjects
	turnNeedsGenerationSub, err := g.natsConn.ChanSubscribe(SubjectTurnNeedsGeneration, needsGenerationChan)
	if err != nil {
		g.log.Err(err).Str("subject", SubjectTurnNeedsGeneration).Msg("failed to subscribe to subject")
		g.signalReady() // signal ready even on failure to prevent blocking waiters
		return
	}
	defer func() {
		_ = turnNeedsGenerationSub.Unsubscribe() // nats connection may already be closed
		// make ourselves as not running just before getting out
		g.running = false
	}()

	// mark ourselves as running just before going into the infinite loop
	g.running = true
	// signal that we are ready
	g.signalReady()
	for {
		select {
		case <-ctx.Done():
			// context is done, goodbye cruel world.
			g.log.Debug().Msg("turnNeedsGeneration subscription shutting down as context has been cancelled")
			return
		case msg := <-needsGenerationChan:
			g.log.Debug().Str("data", string(msg.Data)).Msg("turnNeedsGeneration message received")
			if err := g.needsGeneration(msg); err != nil {
				// no need to log, this is already logged
				continue
			}
		}
	}
}

// Shutdown cancels the goroutine that is Run()ing and waits for it to exit.
// It is safe to call Shutdown multiple times or on an already-stopped generator.
func (g *TurnGenerator) Shutdown() {
	if !g.running {
		return
	}
	g.cancelFunc()
	<-g.done // wait for Run() to fully exit
}

// NeedsGenerationMsg is the message we will receive on the SubjectTurnNeedsGeneration
// NATS.io subject.
type NeedsGenerationMsg struct {
	SessionID string `json:"session_id"`
	Year      int    `json:"year"`
}

// needsGeneration will be called when we receive a message telling us we need to
// generate a new turn
func (g *TurnGenerator) needsGeneration(msg *nats.Msg) error {
	ctx := context.Background()
	var needsGenerationMsg NeedsGenerationMsg
	err := json.Unmarshal(msg.Data, &needsGenerationMsg)
	if err != nil {
		g.log.Err(err).Msg("failed to unmarshal turn needsGenerationMsg")
		return err
	}

	err = g.genTurn(ctx, needsGenerationMsg.SessionID, needsGenerationMsg.Year)
	if err != nil {
		g.log.Err(err).Msg("failed to generate turn with stars.exe")
		return err
	}

	g.log.Debug().Msg("turn generated successfully")

	// Publish to notifications system
	if g.notifyService != nil {
		newYear := int64(needsGenerationMsg.Year + 1)
		_ = g.notifyService.PublishSessionTurnReady(needsGenerationMsg.SessionID, newYear)
	}

	return nil
}

func (g *TurnGenerator) genTurn(ctx context.Context, sessionID string, year int) error {
	logger := *g.log
	tx, err := database.Begin(ctx, g.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(logger)
	sqlH := database.NewSQLHelper(ctx, tx, logger)
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSessionNotFound("session not found: " + sessionID)
		}
		g.log.Err(err).Msg("failed to get session from DB")
		return err
	}

	sessionPlayerRaces, err := sessionDB.SessionPlayerRaces(&sqlH)
	if err != nil {
		g.log.Err(err).Str("sessionID", sessionID).Msg("failed to get player races for session")
		return err
	}

	var sessionFilesDB models.SessionFilesDB
	query := sessionFilesQueryByYear(sessionID, year)
	if err := sqlH.Get(&sessionFilesDB, query); err != nil {
		g.log.Err(err).Str("sessionID", sessionID).Int("year", year).Msg("failed to get sessionFilesDB for the needed year")
		return err
	}
	inputGF, err := NewGameFilesFromSessionFiles(sessionFilesDB.SessionFiles)
	if err != nil {
		g.log.Err(err).Str("sessionID", sessionID).Int("year", year).Msg("failed to get produce game files from sessionfile for the needed year")
		return err
	}

	gf, err := g.runner.GenTurn(ctx, g.log, sessionID, inputGF, sessionPlayerRaces)
	if err != nil {
		g.log.Err(err).Str("sessionID", sessionID).Int("year", year).Msg("failed to get generate turn")
		return err
	}

	// Create new SessionFilesDB for the new turn
	var newSessionFilesDB models.SessionFilesDB
	if err := gf.HydrateSessionFiles(&newSessionFilesDB.SessionFiles); err != nil {
		g.log.Err(err).Str("sessionID", sessionID).Int("year", year).Msg("failed to hydrate session files from gamefiles")
		return err
	}

	id, err := uuid.V4()
	if err != nil {
		return err
	}
	newSessionFilesDB.ID = id.String()
	newSessionFilesDB.SessionID = sessionID

	// Insert the new turn files into the database
	if _, err := sqlH.Insert(&newSessionFilesDB); err != nil {
		g.log.Err(err).Str("sessionID", sessionID).Int("year", year).Msg("failed to insert new session files in database")
		return err
	}

	if err := tx.Commit(); err != nil {
		g.log.Err(err).Str("sessionID", sessionID).Int("year", year).Msg("failed to commit transaction")
		return err
	}

	return nil
}

func sessionFilesQueryByYear(sessionID string, year int) sq.SelectBuilder {
	return database.SQ.
		Select().
		Columns(models.SessionFilesDBColumns...).
		From(models.SessionFilesDBTable).
		Where(
			sq.And{
				sq.Eq{models.SessionFilesDBSessionIDColumn: sessionID},
				sq.Eq{models.SessionFilesDBYearColumn: year},
			},
		)
}

func (r *Runner) GenTurn(ctx context.Context, log *zerolog.Logger, sessionID string, gameFiles *GameFiles, players []models.SessionPlayerRace) (*GameFiles, error) {
	sessionDir := r.localSessionSaveDir(sessionID)
	if err := os.MkdirAll(sessionDir, 0770); err != nil { // #nosec G301 -- dir needs group write for wine
		r.log.Err(err).Str("sessionID", sessionID).Msg("failed to create session dir")
		return nil, err
	}

	if err := r.saveOrderFilesToSessionDir(sessionID, gameFiles.Orders); err != nil {
		log.Err(err).Msg("failed to save player orders to session dir")
		return nil, err
	}

	if err := r.saveUniverseFileToSessionDir(sessionID, gameFiles.Universe); err != nil {
		log.Err(err).Msg("failed to save universe file to session dir")
		return nil, err
	}

	if err := r.saveHostFileToSessionDir(sessionID, gameFiles.HostFile); err != nil {
		log.Err(err).Msg("failed to save host file to session dir")
		return nil, err
	}

	if err := r.newTurn(ctx, sessionID); err != nil {
		return nil, err
	}

	return r.gameFilesForTurn(sessionID, players)
}

func (r *Runner) saveUniverseFileToSessionDir(sessionID string, content []byte) error {
	sessionDir := r.localSessionSaveDir(sessionID)
	return saveUniverseFile(r.log, sessionDir, content)
}

func saveUniverseFile(log *zerolog.Logger, sessionDir string, content []byte) error {
	targetFileName := filepath.Join(sessionDir, universeBaseFilename)
	targetFile, err := os.OpenFile(targetFileName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		log.Err(err).Str("filename", targetFileName).Msg("failed to open universe file for creation")
		return err
	}
	defer func() {
		if err := targetFile.Close(); err != nil {
			log.Err(err).Str("filename", targetFileName).Msg("failed to close universe file")
		}
	}()

	n, err := targetFile.Write(content)
	if err != nil {
		log.Err(err).Str("filename", targetFileName).Msg("failed to write into universe file")
		return err
	}
	log.Debug().Int("# bytes", n).Str("universeFile", targetFileName).Msg("wrote universe file")

	return nil
}

func (r *Runner) saveHostFileToSessionDir(sessionID string, content []byte) error {
	sessionDir := r.localSessionSaveDir(sessionID)
	return saveHostFile(r.log, sessionDir, content)
}

func saveHostFile(log *zerolog.Logger, sessionDir string, content []byte) error {
	targetFileName := filepath.Join(sessionDir, hostBaseFilename)
	targetFile, err := os.OpenFile(targetFileName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		log.Err(err).Str("filename", targetFileName).Msg("failed to open host file for creation")
		return err
	}
	defer func() {
		if err := targetFile.Close(); err != nil {
			log.Err(err).Str("filename", targetFileName).Msg("failed to close host file")
		}
	}()

	n, err := targetFile.Write(content)
	if err != nil {
		log.Err(err).Str("filename", targetFileName).Msg("failed to write into host file")
		return err
	}
	log.Debug().Int("# bytes", n).Str("hostFile", targetFileName).Msg("wrote host file")

	return nil
}

func (r *Runner) saveOrderFilesToSessionDir(sessionID string, orderFiles [][]byte) error {
	sessionDir := r.localSessionSaveDir(sessionID)
	for idx, orderFile := range orderFiles {
		if len(orderFile) != 0 {
			if err := saveOrderFile(r.log, sessionDir, orderFile, idx); err != nil {
				return err
			}
		}
	}
	return nil
}

func saveOrderFile(log *zerolog.Logger, sessionDir string, content []byte, playerOrder int) error {
	targetFileName := filepath.Join(sessionDir, fmt.Sprintf("%s%d", orderBaseFilename, playerOrder+1))
	targetFile, err := os.OpenFile(targetFileName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		log.Err(err).Str("filename", targetFileName).Msg("failed to open order file for creation")
		return err
	}
	defer func() {
		if err := targetFile.Close(); err != nil {
			log.Err(err).Str("filename", targetFileName).Msg("failed to close order file")
		}
	}()

	n, err := targetFile.Write(content)
	if err != nil {
		log.Err(err).Str("filename", targetFileName).Msg("failed to write into order file")
		return err
	}
	log.Debug().Int("# bytes", n).Str("turnFile", targetFileName).Msg("wrote order file")

	return nil
}

func (r *Runner) newTurn(ctx context.Context, sessionID string) error {
	hostWFilePath := r.wPathJoin(r.wSessionSaveDir(sessionID), hostBaseFilename)
	// Load the host file, generate 1 turn and quit.
	// wine stars.exe -g game.hst
	genCmd := cmd.NewCmd(wine, r.wStarsExecutablePath(), "-g", hostWFilePath)
	// inject the wine prefix & display in the env vars
	genCmd.Env = append(genCmd.Env, r.winePrefixEnv(), r.displayEnv())
	stdOut, stdErr, err := RunCMDTimeout(r.log, genCmd, r.CommandsTimeout)
	if err != nil {
		r.log.Err(err).
			Str("sessionID", sessionID).
			Str("stdOut", strings.Join(stdOut, " ")).
			Str("stdErr", strings.Join(stdErr, " ")).
			Msg("failed generate new turn")
		return err
	}
	return nil
}
