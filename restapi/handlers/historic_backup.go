package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewHistoricBackupHandler creates a handler for downloading historic game files as a ZIP
func NewHistoricBackupHandler(log *zerolog.Logger, db *sqlx.DB) *HistoricBackupHandler {
	return &HistoricBackupHandler{log: log, db: db}
}

// HistoricBackupHandler handles GET /sessions/{session_id}/backup
type HistoricBackupHandler struct {
	log *zerolog.Logger
	db  *sqlx.DB
}

// BackupResult holds the result of a backup operation
type BackupResult struct {
	ZipData   *bytes.Buffer
	SessionID string
}

// playerContext holds information about the player's role in the session
type playerContext struct {
	isManager   bool
	playerOrder int64
}

func (h *HistoricBackupHandler) handle(
	ctx context.Context, params operations.HistoricBackupParams, principal *models.Principal,
) (*BackupResult, error) {
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

	// Get player context (manager status, player order)
	pCtx, err := h.getPlayerContext(sqlH, params.SessionID, principal)
	if err != nil {
		return nil, err
	}

	// Get session to check if started
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, params.SessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSessionNotFound("session not found: " + params.SessionID)
		}
		return nil, err
	}

	// Allow backup for started and archived sessions, but not pending
	if sessionDB.State == models.SessionStatePending {
		return nil, errs.NewErrSessionNotStarted("session has not started yet")
	}

	// Get all session files ordered by year
	allSessionFiles, err := h.getAllSessionFiles(ctx, sqlH, params.SessionID)
	if err != nil {
		return nil, err
	}

	// Get race files for the session
	raceFiles, err := h.getSessionRaceFiles(ctx, sqlH, params.SessionID, pCtx.isManager, principal.Subject)
	if err != nil {
		return nil, err
	}

	// Create ZIP file
	zipBuffer, err := h.createZipArchive(allSessionFiles, raceFiles, pCtx.isManager, pCtx.playerOrder)
	if err != nil {
		return nil, err
	}

	return &BackupResult{
		ZipData:   zipBuffer,
		SessionID: params.SessionID,
	}, nil
}

// Handle handles the request
func (h *HistoricBackupHandler) Handle(
	params operations.HistoricBackupParams, principal *models.Principal,
) middleware.Responder {
	result, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewHistoricBackupForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewHistoricBackupNotFound().WithPayload(models.FromError(err))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return operations.NewHistoricBackupPreconditionFailed().WithPayload(models.FromError(err))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}

	return &historicBackupResponse{
		payload:   io.NopCloser(bytes.NewReader(result.ZipData.Bytes())),
		sessionID: result.SessionID,
		log:       h.log,
	}
}

// Authorize checks if user can access the backup
func (h *HistoricBackupHandler) Authorize(
	sqlH database.SQLHelper, params operations.HistoricBackupParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}

	// Check if user is a session member
	var rel models.UserProfileSessionRelDB
	query := userProfileSessionRelationQuery(principal.Subject, params.SessionID)
	if err := sqlH.Get(&rel, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// Session members are authorized (manager or regular member with a race)
	if rel.IsManager {
		return true, nil
	}

	// Regular member needs to have a race set up
	var spr models.SessionPlayerRaceDB
	sprQuery := sessionPlayerRaceQuery(principal.Subject, params.SessionID)
	if err := sqlH.Get(&spr, sprQuery); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Member but hasn't set up a race yet - can't get backup
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// getPlayerContext returns the player's role and order in the session
func (h *HistoricBackupHandler) getPlayerContext(
	sqlH database.SQLHelper, sessionID string, principal *models.Principal,
) (playerContext, error) {
	if principal.IsGlobalManager {
		return playerContext{isManager: true, playerOrder: -1}, nil
	}

	// Check if user is a session manager
	var rel models.UserProfileSessionRelDB
	query := userProfileSessionRelationQuery(principal.Subject, sessionID)
	if err := sqlH.Get(&rel, query); err != nil {
		return playerContext{}, err
	}

	if rel.IsManager {
		return playerContext{isManager: true, playerOrder: -1}, nil
	}

	// Get their player order
	var spr models.SessionPlayerRaceDB
	sprQuery := sessionPlayerRaceQuery(principal.Subject, sessionID)
	if err := sqlH.Get(&spr, sprQuery); err != nil {
		return playerContext{}, err
	}

	return playerContext{isManager: false, playerOrder: spr.PlayerOrder}, nil
}

// getAllSessionFiles retrieves all session files for all years, ordered by year ASC
func (h *HistoricBackupHandler) getAllSessionFiles(
	ctx context.Context, sqlH database.SQLHelper, sessionID string,
) ([]models.SessionFilesDB, error) {
	query := database.SQ.
		Select().
		Columns(models.SessionFilesDBColumns...).
		From(models.SessionFilesDBTable).
		Where(sq.Eq{models.SessionFilesDBSessionIDColumn: sessionID}).
		OrderBy(models.SessionFilesDBYearColumn + " ASC")

	var sessionFiles []models.SessionFilesDB
	if err := sqlH.Select(&sessionFiles, query); err != nil {
		return nil, err
	}

	if len(sessionFiles) == 0 {
		return nil, errs.NewErrSomethingNotFound("no session files found for session: " + sessionID)
	}

	return sessionFiles, nil
}

// raceFileInfo holds race file data with player order
type raceFileInfo struct {
	PlayerOrder int64
	Data        string // base64 encoded
}

// getSessionRaceFiles retrieves race files for players in the session
func (h *HistoricBackupHandler) getSessionRaceFiles(
	ctx context.Context, sqlH database.SQLHelper, sessionID string, isManager bool, userProfileID string,
) ([]raceFileInfo, error) {
	// Get session player races
	query := database.SQ.
		Select().
		Columns(models.SessionPlayerRaceDBColumns...).
		From(models.SessionPlayerRaceDBTable).
		Where(sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID}).
		OrderBy(models.SessionPlayerRaceDBPlayerOrderColumn + " ASC")

	var sprs []models.SessionPlayerRaceDB
	if err := sqlH.Select(&sprs, query); err != nil {
		return nil, err
	}

	var raceFiles []raceFileInfo
	for _, spr := range sprs {
		if spr.IsBot {
			continue // Bots don't have user-uploaded race files
		}

		// If not manager, only get the user's own race
		if !isManager && spr.UserProfileID != userProfileID {
			continue
		}

		// Get the race data
		var raceDB models.RaceDB
		if err := sqlH.GetByPKey(&raceDB, spr.RaceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Race was deleted, skip
				continue
			}
			return nil, err
		}

		raceFiles = append(raceFiles, raceFileInfo{
			PlayerOrder: spr.PlayerOrder,
			Data:        raceDB.Data,
		})
	}

	return raceFiles, nil
}

// createZipArchive creates a ZIP archive with all requested files
func (h *HistoricBackupHandler) createZipArchive(
	sessionFiles []models.SessionFilesDB,
	raceFiles []raceFileInfo,
	isManager bool,
	playerOrder int64,
) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	for _, sf := range sessionFiles {
		year := sf.Year

		// Universe file (.xy) - everyone gets this
		if sf.Universe != "" {
			if err := h.addFileToZip(zw, fmt.Sprintf("game-%d.xy", year), sf.Universe); err != nil {
				return nil, err
			}
		}

		// Host file (.hst) - managers only
		if isManager && sf.HostFile != "" {
			if err := h.addFileToZip(zw, fmt.Sprintf("game-%d.hst", year), sf.HostFile); err != nil {
				return nil, err
			}
		}

		// Turn files (.mN)
		for idx, turn := range sf.Turns {
			if turn.B64Data == "" {
				continue
			}
			// Managers get all turns, regular players only get their own
			if isManager || int64(idx) == playerOrder {
				filename := fmt.Sprintf("game-%d.m%d", year, idx+1)
				if err := h.addFileToZip(zw, filename, turn.B64Data); err != nil {
					return nil, err
				}
			}
		}
	}

	// Race files (.rN) - from the first year only (races don't change)
	for _, rf := range raceFiles {
		filename := fmt.Sprintf("game.r%d", rf.PlayerOrder+1)
		if err := h.addFileToZip(zw, filename, rf.Data); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf, nil
}

// addFileToZip adds a base64-encoded file to the ZIP archive
func (h *HistoricBackupHandler) addFileToZip(zw *zip.Writer, filename, b64data string) error {
	data, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		h.log.Warn().Str("filename", filename).Err(err).Msg("failed to decode base64 data for zip")
		return err
	}

	fw, err := zw.Create(filename)
	if err != nil {
		return err
	}

	_, err = fw.Write(data)
	return err
}

// historicBackupResponse is a custom responder for ZIP file download
type historicBackupResponse struct {
	payload   io.ReadCloser
	sessionID string
	log       *zerolog.Logger
}

func (r *historicBackupResponse) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {
	rw.Header().Set("Content-Type", "application/zip")
	rw.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-backup.zip"`, r.sessionID))
	rw.WriteHeader(200)
	if r.payload != nil {
		defer func() {
			if err := r.payload.Close(); err != nil {
				r.log.Err(err).Msg("failed to close payload reader")
			}
		}()
		_, _ = io.Copy(rw, r.payload)
	}
}
