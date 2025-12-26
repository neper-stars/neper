package stars

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"github.com/neper-stars/neper/lib/racefiles"
	"github.com/neper-stars/neper/models"
)

const (
	gameDefName = "game.def"
)

// wSessionSaveDir returns a "windows" directory name for the session save dir
func (r *Runner) wSessionSaveDir(sessionID string) string {
	return saveDirDriveLetter + windowsSep + sessionID
}

// localSessionSaveDir return a local directory name for the session save dir
func (r *Runner) localSessionSaveDir(sessionID string) string {
	return filepath.Join(r.SaveDir, sessionID)
}

// wStarsExecutablePath return a "windows" full path for the stars executable
// ie: something like `s:\stars.exe`
func (r *Runner) wStarsExecutablePath() string {
	return r.wPathJoin(executableDirDriveLetter, starsExecutableName)
}

// wPathJoin takes multiple path segments and returns a joined result in the
// windows fashion.
// ie: dir1 + dir2 + file.txt => `dir1\dir2\file.txt`
func (r *Runner) wPathJoin(segments ...string) string {
	var segs []string
	segs = append(segs, segments...)
	return strings.Join(segs, windowsSep)
}

func (r *Runner) closeDeferred(closer io.ReadCloser) {
	if err := closer.Close(); err != nil {
		r.log.Err(err).Msg("failed to close resource")
	}
}

// gameFilesForTurn will return a *GameFiles struct with all files for neper saving and redistributing
// this *GamesFiles struct will contain only Turns and nothing in Orders. (hence the name)
func (r *Runner) gameFilesForTurn(sessionID string, players []models.SessionPlayerRace) (*GameFiles, error) {
	gf := GameFiles{}
	sessionDir := r.localSessionSaveDir(sessionID)
	// here we go, first read the .xy file which should be named game.xy (universeBaseFilename)
	universe, err := os.Open(filepath.Join(sessionDir, universeBaseFilename)) // #nosec G304 -- path from internal config
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
	pTurnFileName := filepath.Join(sessionDir, turnBaseFilename+fmt.Sprintf("%d", player.PlayerOrder+1))
	playerTurn, err := os.Open(pTurnFileName)
	if err != nil {
		r.log.Err(err).
			Int64("player", player.PlayerOrder).
			Str("file", pTurnFileName).
			Msg("failed to open player turn file")
		return nil, err
	}
	defer r.closeDeferred(playerTurn)
	playerTurnContent, err := io.ReadAll(playerTurn)
	if err != nil {
		return nil, err
	}
	return playerTurnContent, nil
}

type RaceFile struct {
	PlayerOrder int64
	Data        []byte
}

type RaceFiles []RaceFile

func (r *Runner) NewGame(ctx context.Context, log *zerolog.Logger, sessionID string, gameInput *GameInput, players []models.SessionPlayerRace, races []models.Race) (*GameFiles, error) {
	content, err := gameInput.Content()
	if err != nil {
		log.Err(err).Msg("failed to generate game input to create new stars game")
		return nil, err
	}

	sessionDir := r.localSessionSaveDir(sessionID)
	if err := os.MkdirAll(sessionDir, 0770); err != nil { // #nosec G301 -- dir needs group write for wine
		r.log.Err(err).Str("sessionID", sessionID).Msg("failed to create session dir")
		return nil, err
	}

	raceFiles, err := r.newRaceFiles(players, races)
	if err != nil {
		log.Err(err).Msg("failed to generate race map")
		return nil, err
	}

	if err := r.saveRaceFilesToSessionDir(sessionID, raceFiles); err != nil {
		r.log.Err(err).Msg("failed to save race files to session dir before new game generation")
		return nil, err
	}

	if err := r.newGame(ctx, sessionID, content); err != nil {
		return nil, err
	}
	return r.gameFilesForTurn(sessionID, players)
}

func (r *Runner) newRaceFiles(players []models.SessionPlayerRace, races []models.Race) (RaceFiles, error) {
	var rf RaceFiles
	for _, player := range players {
		if player.IsBot {
			// bots do not have race files in gameinput definitions
			// they only play predefined races
			continue
		}
		var playerRaceFound bool
		for _, race := range races {
			if race.ID == player.RaceID {
				// found proper race
				playerRaceFound = true
				data, err := race.RawData()
				if err != nil {
					r.log.Err(err).Msg("failed to read data from race model")
					return nil, err
				}

				rf = append(rf, RaceFile{
					PlayerOrder: player.PlayerOrder,
					Data:        data,
				})
				break
			}
		}
		if !playerRaceFound {
			r.log.Error().Str("playerID", player.ID).Str("raceID", player.RaceID).Msg("player race not found")
			return nil, errors.New("player race not found")
		}
	}
	return rf, nil
}

func (r *Runner) saveRaceFilesToSessionDir(sessionID string, rf RaceFiles) error {
	sessionDir := r.localSessionSaveDir(sessionID)
	for i := range rf {
		if err := r.saveRaceFile(sessionDir, rf[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) saveRaceFile(sessionDir string, raceFile RaceFile) error {
	raceFileName := filepath.Join(sessionDir, fmt.Sprintf("game.r%d", raceFile.PlayerOrder+1))
	targetRace, err := os.OpenFile(raceFileName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		r.log.Err(err).Str("filename", raceFileName).Msg("failed to open race file for creation")
		return err
	}
	defer func() {
		if err := targetRace.Close(); err != nil {
			r.log.Err(err).Str("filename", raceFileName).Msg("failed to close race file")
		}
	}()

	// Process race file using the racefiles processor
	opts := racefiles.ProcessorOptions{
		StripPassword: r.stripRacePasswords,
		FixCorrupted:  r.fixRaceFiles,
	}
	data, analysis := racefiles.ProcessData(r.log, raceFile.Data, opts)
	racefiles.LogAnalysis(r.log, analysis, raceFileName)

	n, err := targetRace.Write(data)
	if err != nil {
		r.log.Err(err).Str("filename", raceFileName).Msg("failed to write into race file")
		return err
	}
	r.log.Debug().Int("# bytes", n).Str("racefile", raceFileName).Msg("wrote race file")

	return nil
}

func (r *Runner) newGame(ctx context.Context, sessionID string, content io.Reader) error {
	sessionDir := r.localSessionSaveDir(sessionID)
	localPath := filepath.Join(sessionDir, gameDefName)
	targetGameDef, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		r.log.Err(err).Str("sessionID", sessionID).Msg("failed to create game def for session new game")
		return err
	}
	defer func() {
		if err := targetGameDef.Close(); err != nil {
			r.log.Err(err).Str("sessionID", sessionID).Msg("failed to close game.def after writing to it")
		}
	}()
	_, err = io.Copy(targetGameDef, content)
	if err != nil {
		r.log.Err(err).Str("sessionID", sessionID).Msg("failed to write: game.def")
		return err
	}
	// ensure content is on disk before calling stars on the file
	if err := targetGameDef.Sync(); err != nil {
		r.log.Err(err).Str("sessionID", sessionID).Msg("failed to sync to disk: game.def")
		return err
	}

	gameDefWindowsPath := r.wPathJoin(r.wSessionSaveDir(sessionID), gameDefName)
	// use content to create a new game
	stdOut, stdErr, err := r.prefix.RunWine(r.wStarsExecutablePath(), "-a", gameDefWindowsPath)
	if err != nil {
		r.log.Err(err).
			Str("sessionID", sessionID).
			Str("stdOut", strings.Join(stdOut, " ")).
			Str("stdErr", strings.Join(stdErr, " ")).
			Msg("failed generate new game")
		return err
	}
	return nil
}
