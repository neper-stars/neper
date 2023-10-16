package stars

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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

type GameFiles struct {
	Universe []byte   // one game.xy file (the universe and rules, everyone should receive this one)
	HostFile []byte   // one game.hst file (this is the host control file, only for Neper)
	Turns    [][]byte // one game.mX file for each player (including computer players) X should be the player number +1
	Orders   [][]byte // one .rX file for each player (only for the non computer players) X should be the player number +1
}

func b64encode(in []byte) string {
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(in)))
	base64.StdEncoding.Encode(dst, in)
	return string(dst)
}

func (g GameFiles) HydrateSessionFilesDB(s *models.SessionFilesDB) {
	s.Universe = b64encode(g.Universe)
	s.HostFile = b64encode(g.HostFile)

	var turns []*models.Turn
	var turnsDB []models.Turn
	for i := range g.Turns {
		encoded := b64encode(g.Turns[i])
		turn := models.Turn{B64Data: &encoded}
		turns = append(turns, &turn)
		turnsDB = append(turnsDB, turn)
	}
	s.Turns = turns
	s.TurnsDB = turnsDB

	var orders []*models.Order
	var ordersDB []models.Order
	for i := range g.Orders {
		encoded := b64encode(g.Orders[i])
		order := models.Order{B64Data: &encoded}
		orders = append(orders, &order)
		ordersDB = append(ordersDB, order)
	}
	s.Orders = orders
	s.OrdersDB = ordersDB
}

func (r *Runner) closeDeferred(closer io.ReadCloser) {
	if err := closer.Close(); err != nil {
		r.log.Err(err).Msg("failed to close resource")
	}
}

// NewGameFilesForTurn will return a *GameFiles struct with all files for neper saving and redistributing
// this *GamesFiles struct will contain only Turns and nothing in Orders. (hence the name)
func (r *Runner) NewGameFilesForTurn(sessionID string, players []models.SessionPlayerRace) (*GameFiles, error) {
	gf := GameFiles{}
	sessionDir := r.localSessionSaveDir(sessionID)
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
	if err := os.MkdirAll(sessionDir, 0770); err != nil {
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
	return r.NewGameFilesForTurn(sessionID, players)
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
		if err := saveRaceFile(r.log, sessionDir, rf[i]); err != nil {
			return err
		}
	}
	return nil
}

func saveRaceFile(log *zerolog.Logger, sessionDir string, raceFile RaceFile) error {
	raceFileName := filepath.Join(sessionDir, fmt.Sprintf("game.r%d", raceFile.PlayerOrder+1))
	targetRace, err := os.OpenFile(raceFileName, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		log.Err(err).Str("filename", raceFileName).Msg("failed to open race file for creation")
		return err
	}
	defer func() {
		if err := targetRace.Close(); err != nil {
			log.Err(err).Str("filename", raceFileName).Msg("failed to close race file")
		}
	}()

	n, err := targetRace.Write(raceFile.Data)
	if err != nil {
		log.Err(err).Str("filename", raceFileName).Msg("failed to write into race file")
		return err
	}
	log.Debug().Int("# bytes", n).Str("racefile", raceFileName).Msg("wrote race file")

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
	genCmd := cmd.NewCmd(wine, r.wStarsExecutablePath(), "-a", gameDefWindowsPath)
	// inject the wine prefix & display in the env vars
	genCmd.Env = append(genCmd.Env, r.winePrefixEnv(), r.displayEnv())
	stdOut, stdErr, err := RunCMDTimeout(r.log, genCmd, r.CommandsTimeout)
	if err != nil {
		r.log.Err(err).
			Str("sessionID", sessionID).
			Str("stdOut", strings.Join(stdOut, " ")).
			Str("stdErr", strings.Join(stdErr, " ")).
			Msg("failed generate new game")
	}
	return nil
}
