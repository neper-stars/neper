package stars

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-cmd/cmd"
	"github.com/rs/zerolog"

	"fmt"

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

type GameFiles struct {
	Universe []byte   // one game.xy file (the universe and rules, everyone should receive this one)
	HostFile []byte   // one game.hst file (this is the host control file, only for Neper)
	Turns    [][]byte // one game.mX file for each player (including computer players) X should be the player number +1
	Orders   [][]byte // one .rX file for each player (only for the non computer players) X should be the player number +1
}

func (r *Runner) closeDeferred(closer io.ReadCloser) {
	if err := closer.Close(); err != nil {
		r.log.Err(err).Msg("failed to close resource")
	}
}

// NewGameFilesForTurn will return a *GameFiles struct with all files for neper saving and redistributing
// this *GamesFiles struct will contain only Turns and nothing in Orders. (hence the name)
func (r *Runner) NewGameFilesForTurn(session models.Session, players []models.SessionPlayerRace) (*GameFiles, error) {
	gf := GameFiles{}
	sessionDir := r.localSessionSaveDir(session.ID)
	// here we go, first read the .xy file which should be named game.xy (universeBaseFilename)
	universe, err := os.Open(filepath.Join(sessionDir, universeBaseFilename))
	if err != nil {
		return nil, err
	}
	defer r.closeDeferred(universe)

	universeContent, err := io.ReadAll(universe)
	if err != nil {
		return nil, err
	}
	gf.Universe = universeContent

	hostFile, err := os.Open(filepath.Join(sessionDir, hostBaseFilename))
	if err != nil {
		return nil, err
	}
	defer r.closeDeferred(hostFile)
	hostContent, err := io.ReadAll(hostFile)
	if err != nil {
		return nil, err
	}
	gf.HostFile = hostContent

	for _, player := range players {
		if player.IsBot {
			continue
		}
		playerTurnContent, err := r.readPlayerTurn(sessionDir, player)
		if err != nil {
			r.log.Err(err).
				Str("player", player.ID).
				Int64("order", player.PlayerOrder).
				Msg("failed to read player turn")
			return nil, err
		}
		gf.Turns = append(gf.Turns, playerTurnContent)
	}
	return &gf, nil
}

func (r *Runner) readPlayerTurn(sessionDir string, player models.SessionPlayerRace) ([]byte, error) {
	playerTurn, err := os.Open(filepath.Join(sessionDir, turnBaseFilename+fmt.Sprintf("%d", player.PlayerOrder)))
	if err != nil {
		return nil, err
	}
	defer r.closeDeferred(playerTurn)
	playerTurnContent, err := io.ReadAll(playerTurn)
	if err != nil {
		return nil, err
	}
	return playerTurnContent, nil
}

func (r *Runner) NewGame(ctx context.Context, log *zerolog.Logger, session models.Session, ruleset models.Ruleset, players []models.SessionPlayerRace) (*GameFiles, error) {
	gameInput := NewGameInput(log, saveDirDriveLetter, session.ID, session.Name, ruleset, players)
	content, err := gameInput.Content()
	if err != nil {
		log.Err(err).Msg("failed to generate game input to create new stars game")
		return nil, err
	}
	if err := r.newGame(ctx, log, session, content); err != nil {
		return nil, err
	}
	return r.NewGameFilesForTurn(session, players)
}

func (r *Runner) newGame(ctx context.Context, log *zerolog.Logger, session models.Session, content io.Reader) error {

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
	// inject the wine prefix & display in the env vars
	genCmd.Env = append(genCmd.Env, r.winePrefixEnv(), r.displayEnv())
	stdOut, stdErr, err := RunCMDTimeout(r.log, genCmd, r.CommandsTimeout)
	if err != nil {
		r.log.Err(err).
			Str("sessionID", session.ID).
			Str("stdOut", strings.Join(stdOut, " ")).
			Str("stdErr", strings.Join(stdErr, " ")).
			Msg("failed generate new game")
	}
	return nil
}
