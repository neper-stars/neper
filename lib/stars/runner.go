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

	"time"

	"strings"

	"github.com/go-cmd/cmd"
	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
)

const (
	wine                     = "wine"
	xvfb                     = "Xvfb"
	wineHostname             = "hostname"
	dosDevicesDir            = "dosdevices"
	saveDirDriveLetter       = "s:"
	executableDirDriveLetter = "x:"
	starsExecutableName      = "stars.exe"
)

type RunnerOptions struct {
	ExecutableDir   string `long:"stars-executable-dir" env:"STARS_EXECUTABLE_DIR" ini-name:"stars_executable_dir" description:"directory that will be used to put stars.exe"`
	SaveDir         string `long:"stars-save-dir" env:"STARS_SAVE_DIR" ini-name:"stars_save_dir" description:"directory that will be used as a base to create all savegame dirs"`
	WinePrefix      string `long:"wine-prefix" env:"WINE_PREFIX" ini-name:"wine_prefix" description:"wine prefix to use for running wine apps" default:"~/.wine"`
	CommandsTimeout int    `long:"wine-commands-timeout" env:"WINE_COMMMANDS_TIMEOUT" ini-name:"wine_commands_timeout" description:"time in seconds after which the wine process is considered unresponsive and killed" default:"30"`
	DisplayNumber   int    `long:"display" env:"DISPLAY" ini-name:"display" description:"the display number to provide for wine commands" default:"99"`
}

// NewRunnerOptions ...
func NewRunnerOptions() *RunnerOptions {
	options := RunnerOptions{}
	return &options
}

type Runner struct {
	log             *zerolog.Logger
	ExecutableDir   string // will be mapped to x: for executables
	SaveDir         string // will be mapped to s: for saves
	WinePrefix      string // contains the wine prefix to use
	CommandsTimeout time.Duration
	DisplayNumber   int
	xvfbProcess     *cmd.Cmd // the xvfbProcess we launch at startup
	xvfbStatusChan  <-chan cmd.Status
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
	prefix, err := homedir.Expand(opts.WinePrefix)
	if err != nil {
		log.Err(err).Str("prefix", opts.WinePrefix).Msg("failed to expand given wine prefix")
		return nil, err
	}

	commandsTimout := time.Duration(opts.CommandsTimeout) * time.Second
	return &Runner{
		log:             log,
		ExecutableDir:   absExecutableDir,
		SaveDir:         absSaveDir,
		WinePrefix:      prefix,
		CommandsTimeout: commandsTimout,
		DisplayNumber:   opts.DisplayNumber,
	}, nil
}

func (r *Runner) displayName() string {
	return fmt.Sprintf(":%d", r.DisplayNumber)
}

func (r *Runner) Initialize() error {
	// we don't need a big screen...
	xvfbArgs := []string{r.displayName(), "-screen", "0", "1024x768x16"}
	r.xvfbProcess = cmd.NewCmd(xvfb, xvfbArgs...)
	r.log.Debug().Msg("starting Xvfb virtual X server... giving it 3 seconds")
	r.xvfbStatusChan = r.xvfbProcess.Start()
	wait := time.After(3 * time.Second)
	select {
	case <-wait:
		if r.xvfbProcess.Status().PID != 0 {
			r.log.Debug().
				Int("PID", r.xvfbProcess.Status().PID).
				Msg("started Xvfb")
			break
		}
		r.log.Error().Msg("failed to start Xvfb, timed-out")
	}
	return nil
}

func (r *Runner) Shutdown() {
	pid := r.xvfbProcess.Status().PID
	r.log.Debug().Msg("shutting down Xvfb server")
	if err := r.xvfbProcess.Stop(); err != nil {
		r.log.Err(err).Msg("failed to shutdown Xvfb process")
	}
	// give a small amount of time to the X server before going down
	timeout := time.After(3 * time.Second)
	var gotStatus cmd.Status
	select {
	case gotStatus = <-r.xvfbStatusChan:
	case <-timeout:
		r.log.Error().Msg("error waiting for Xvfb to stop")
		return
	}
	if gotStatus.StopTs > 0 {
		r.log.Debug().Int("PID", pid).Msg("Xvfb server down")
		return
	}
	r.log.Warn().Msg("Xvfb server may not have shutdown properly, please inspect")
}

// PreChecks are specific checks that need to be OK before running initialize
func (r *Runner) PreChecks() error {
	if err := r.ensureXvfb(); err != nil {
		r.log.Err(err).Msg("Xvfb not found in your PATH or neper has no right to execute it.")
		return err
	}
	return nil
}

func (r *Runner) InitialChecks() error {
	if err := r.ensureWine(); err != nil {
		r.log.Err(err).Msg("wine not found in your PATH or neper has no right to execute it.")
		return err
	}
	if err := r.ensureWinePrefix(); err != nil {
		r.log.Err(err).Msg("wineprefix directory not properly configured")
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

func (r *Runner) ensureXvfb() error {
	return CheckFileExecutable(xvfb, true)
}

func (r *Runner) wineConfDir() (string, error) {
	return homedir.Expand(r.WinePrefix)
}

func (r *Runner) devicesDir() (string, error) {
	wd, err := r.wineConfDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, dosDevicesDir), nil
}

func (r *Runner) ensureDriveLetters() error {
	r.log.Debug().Str("letter", saveDirDriveLetter).Msg("checking drive letter")
	if err := r.ensureDriveLetter(saveDirDriveLetter, r.SaveDir); err != nil {
		r.log.Err(err).Str("letter", saveDirDriveLetter).Msg("failed")
		return err
	}
	r.log.Debug().Str("letter", executableDirDriveLetter).Msg("checking drive letter")
	if err := r.ensureDriveLetter(executableDirDriveLetter, r.ExecutableDir); err != nil {
		r.log.Err(err).Str("letter", executableDirDriveLetter).Msg("failed")
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

func (r *Runner) ensureWinePrefix() error {
	info, err := os.Stat(r.WinePrefix)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// wineprefix does not exist yet... we should run `wine winecfg` to initialize it
			r.log.Info().Msg("initializing wineprefix... this can take some time")
			return r.createWinePrefix()
		}
		r.log.Err(err).Str("wineprefix", r.WinePrefix).Msg("failed to stat wineprefix")
		return err
	}
	if info.IsDir() {
		return nil
	}
	// we have a problem.
	return errors.New("wineprefix does not appear to be a directory. Please investigate")
}

func (r *Runner) winePrefixEnv() string {
	return "WINEPREFIX=" + r.WinePrefix
}

func (r *Runner) displayEnv() string {
	return "DISPLAY=" + r.displayName()
}

func (r *Runner) createWinePrefix() error {
	// we run wineHostname as it is a non GUI command and running such a wine command will
	// create the wine prefix we want
	c := cmd.NewCmd(wine, wineHostname)
	// inject wineprefix
	c.Env = append(c.Env, r.winePrefixEnv(), r.displayEnv())
	// unset wine debug if level is not appropriate
	if r.log.GetLevel() > zerolog.DebugLevel {
		// remove all debug from wine
		c.Env = append(c.Env, "WINEDEBUG=-all")
	}
	stdOut, stdErr, err := RunCMDTimeout(r.log, c, r.CommandsTimeout)
	if err != nil {
		msg := ""
		for _, s := range stdOut {
			msg += s
		}
		for _, s := range stdErr {
			msg += s
		}
		r.log.Err(err).Msg(msg)
		return err
	}
	r.log.Debug().
		Str("out", strings.Join(stdOut, "\n")).
		Str("err", strings.Join(stdErr, "\n")).
		Msg("...")
	return nil
}
