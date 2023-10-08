package stars

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"bytes"
	"io"

	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
)

const (
	wine                     = "wine"
	wineCfdDir               = "~/.wine"
	dosDevicesDir            = "dosdevices"
	saveDirDriveLetter       = "s:"
	executableDirDriveLetter = "x:"
	starsExecutableName      = "stars.exe"
)

type RunnerOptions struct {
	ExecutableDir string `long:"stars-executable-dir" ini-name:"stars_executable_dir" description:"directory that will be used to put stars.exe"`
	SaveDir       string `long:"stars-save-dir" ini-name:"stars_save_dir" description:"directory that will be used as a base to create all savegame dirs"`
}

// NewRunnerOptions ...
func NewRunnerOptions() *RunnerOptions {
	options := RunnerOptions{}
	return &options
}

type Runner struct {
	log           *zerolog.Logger
	executableDir string // x: for executables
	saveDir       string // s: for saves
}

func NewRunner(log *zerolog.Logger, executableDir, saveDir string) *Runner {
	return &Runner{
		log:           log,
		executableDir: executableDir,
		saveDir:       saveDir,
	}
}

func (r *Runner) InitialChecks() error {
	if err := r.ensureWine(); err != nil {
		r.log.Err(err).Msg("wine not found in your PATH or neper has no right to execute it.")
		return err
	}
	if err := r.ensureDriveLetters(); err != nil {
		r.log.Err(err).Msg("could not setup required drive letters for wine")
		return err
	}
	if err := r.ensureStars(); err != nil {
		r.log.Err(err).Msg("stars could not be setup in its directory")
		return err
	}
	if err := r.ensureSaveDir(); err != nil {
		r.log.Err(err).Msg("savedir could not be setup")
		return err
	}
	return nil
}

func (r *Runner) ensureWine() error {
	return CheckFileExecutable(wine, true)
}

func (r *Runner) wineConfDir() (string, error) {
	return homedir.Expand(wineCfdDir)
}

func (r *Runner) devicesDir() (string, error) {
	wd, err := r.wineConfDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, dosDevicesDir), nil
}

func (r *Runner) ensureDriveLetters() error {
	if err := r.ensureDriveLetter(saveDirDriveLetter, r.saveDir); err != nil {
		return err
	}
	if err := r.ensureDriveLetter(executableDirDriveLetter, r.executableDir); err != nil {
		return err
	}
	return nil
}

func (r *Runner) ensureDriveLetter(letter, targetDir string) error {
	devicesDir, err := r.devicesDir()
	if err != nil {
		return err
	}
	fInfo, err := os.Lstat(filepath.Join(devicesDir, letter))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// some real error occurred
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		// file does not exist... we will create it
		if err2 := r.createDriveLetter(targetDir, devicesDir, letter); err != nil {
			return err2
		}
	}
	m := fInfo.Mode()
	if !(uint32(m&fs.ModeSymlink) == 0) {
		// file is NOT a symlink
		return errors.New(fmt.Sprintf("%s should be a symlink, not a directory", letter))
	}
	return nil
}

func (r *Runner) createDriveLetter(targetDir, dir, letter string) error {
	return os.Symlink(targetDir, filepath.Join(dir, letter))
}

//go:embed resources/stars26jrc4.exe
var starsBin []byte

func (r *Runner) ensureSaveDir() error {
	if err := os.MkdirAll(r.saveDir, 0770); err != nil {
		return err
	}
	return nil
}

func (r *Runner) ensureStars() error {
	if err := os.MkdirAll(r.executableDir, 0770); err != nil {
		return err
	}
	starsFilePath := filepath.Join(r.executableDir, starsExecutableName)
	_, err := os.Stat(starsFilePath)
	if errors.Is(err, os.ErrNotExist) {
		// we should create the stars.exe file
		// make sure it is not readable / writeable by anyone
		targetStars, err := os.OpenFile(starsFilePath, os.O_RDWR|os.O_CREATE, 0660)
		if err != nil {
			return err
		}
		defer func() {
			if err := targetStars.Close(); err != nil {
				r.log.Err(err).Msg("failed to close stars.exe after writing to it")
			}
		}()
		starsReader := bytes.NewReader(starsBin)
		_, err2 := io.Copy(targetStars, starsReader)
		if err2 != nil {
			r.log.Err(err2).Msg("failed to write into stars.exe")
			return err2
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// a real error occurred here
		return err
	}
	return nil
}
