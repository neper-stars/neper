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
	ExecutableDir string `long:"stars-executable-dir" env:"STARS_EXECUTABLE_DIR" ini-name:"stars_executable_dir" description:"directory that will be used to put stars.exe"`
	SaveDir       string `long:"stars-save-dir" env:"STARS_SAVE_DIR" ini-name:"stars_save_dir" description:"directory that will be used as a base to create all savegame dirs"`
	WinePrefix    string `long:"wine-prefix" env:"WINE_PREFIX" ini-name:"wine_prefix" description:"wine prefix to use for running wine apps" default:"~/.wine"`
}

// NewRunnerOptions ...
func NewRunnerOptions() *RunnerOptions {
	options := RunnerOptions{}
	return &options
}

type Runner struct {
	log           *zerolog.Logger
	ExecutableDir string // will be mapped to x: for executables
	SaveDir       string // will be mapped to s: for saves
	WinePrefix    string // contains the wine prefix to use
}

func NewRunner(log *zerolog.Logger, opts *RunnerOptions) (*Runner, error) {
	absExecutableDir, err := filepath.Abs(opts.ExecutableDir)
	if err != nil {
		return nil, err
	}
	absSaveDir, err := filepath.Abs(opts.SaveDir)
	if err != nil {
		return nil, err
	}
	return &Runner{
		log:           log,
		ExecutableDir: absExecutableDir,
		SaveDir:       absSaveDir,
		WinePrefix:    opts.WinePrefix,
	}, nil
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
	if err := r.ensureDriveLetter(saveDirDriveLetter, r.SaveDir); err != nil {
		return err
	}
	if err := r.ensureDriveLetter(executableDirDriveLetter, r.ExecutableDir); err != nil {
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
	if m.IsDir() {
		return errors.New(fmt.Sprintf("%s should be a symlink, not a directory", letter))
	}
	if m&fs.ModeSymlink != 0 {
		// file IS a symlink
		return nil
	}
	return errors.New(fmt.Sprintf("%s should be a symlink, not a normal file", letter))
}

func (r *Runner) createDriveLetter(targetDir, dir, letter string) error {
	return os.Symlink(targetDir, filepath.Join(dir, letter))
}

//go:embed resources/stars26jrc4.exe
var starsBin []byte

func (r *Runner) ensureSaveDir() error {
	if err := os.MkdirAll(r.SaveDir, 0770); err != nil {
		return err
	}
	return nil
}

func (r *Runner) ensureStars() error {
	if err := os.MkdirAll(r.ExecutableDir, 0770); err != nil {
		return err
	}
	starsFilePath := filepath.Join(r.ExecutableDir, starsExecutableName)
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
