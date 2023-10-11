package stars

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-cmd/cmd"
	"github.com/rs/zerolog"

	"github.com/neper-stars/neper/models"
)

const (
	gameDefName = "game.def"
)

func (r *Runner) wSessionSaveDir(sessionID string) string {
	return saveDirDriveLetter + windowsSep + sessionID
}

func (r *Runner) localSessionSaveDir(sessionID string) string {
	return filepath.Join(r.SaveDir, sessionID)
}

func (r *Runner) wStarsExecutablePath() string {
	return r.wPathJoin(executableDirDriveLetter, starsExecutableName)
}

func (r *Runner) wPathJoin(segments ...string) string {
	var segs []string
	segs = append(segs, segments...)
	return strings.Join(segs, windowsSep)
}

func (r *Runner) NewGame(ctx context.Context, log *zerolog.Logger, session models.Session, ruleset models.Ruleset, players []models.SessionPlayerRace) error {
	gameInput := NewGameInput(log, saveDirDriveLetter, session.ID, session.Name, ruleset, players)
	content, err := gameInput.Content()
	if err != nil {
		log.Err(err).Msg("failed to generate game input to create new stars game")
		return err
	}

	localPath := filepath.Join(r.localSessionSaveDir(session.ID), gameDefName)
	targetGameDef, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	defer func() {
		if err := targetGameDef.Close(); err != nil {
			r.log.Err(err).Str("sessionID", session.ID).Msg("failed to close game.def after writing to it")
		}
	}()
	_, err = io.Copy(targetGameDef, content)
	if err != nil {
		r.log.Err(err).Str("sessionID", session.ID).Msg("failed to write: game.def")
		return err
	}
	// ensure content is on disk before calling stars on the file
	if err := targetGameDef.Sync(); err != nil {
		r.log.Err(err).Str("sessionID", session.ID).Msg("failed to sync to disk: game.def")
		return err
	}

	gameDefWindowsPath := r.wPathJoin(r.wSessionSaveDir(session.ID), gameDefName)
	// use content to create a new game
	genCmd := cmd.NewCmd(wine, r.wStarsExecutablePath(), "-a", gameDefWindowsPath)
	// inject the wine prefix in the env vars
	genCmd.Env = append(genCmd.Env, r.WinePrefix)
	stdOut, stdErr, err := RunCMD(genCmd)
	if err != nil {
		r.log.Err(err).
			Str("sessionID", session.ID).
			Str("stdOut", strings.Join(stdOut, " ")).
			Str("stdErr", strings.Join(stdErr, " ")).
			Msg("failed generate new game")
	}
	return nil
}
