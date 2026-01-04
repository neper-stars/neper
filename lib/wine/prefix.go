//go:build linux

package wine

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
)

const (
	wineBin        = "wine"
	winebootBin    = "wineboot"
	wineserverBin  = "wineserver"
	winebootInit   = "init"
	wineserverKill = "-k"
	wineserverWait = "-w"
	dosDevicesDir  = "dosdevices"
)

// PrefixOptions contains configuration for a wine prefix
type PrefixOptions struct {
	// PrefixPath is the path to the wine prefix directory (e.g., ~/.wine)
	PrefixPath string
	// CommandsTimeout is the maximum time to wait for wine commands to complete
	CommandsTimeout time.Duration
	// Display is the X display to use (e.g., ":99"). If empty, DISPLAY env var is not set.
	Display string
}

// Prefix manages a wine prefix directory
type Prefix struct {
	path    string // absolute path to wine prefix
	timeout time.Duration
	display string
	log     *zerolog.Logger
}

// NewPrefix creates a new Prefix instance
func NewPrefix(log *zerolog.Logger, opts PrefixOptions) (*Prefix, error) {
	absPath, err := expand(opts.PrefixPath)
	if err != nil {
		return nil, err
	}

	timeout := opts.CommandsTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Prefix{
		path:    absPath,
		timeout: timeout,
		display: opts.Display,
		log:     log,
	}, nil
}

// Path returns the absolute path to the wine prefix
func (p *Prefix) Path() string {
	return p.path
}

// Env returns the environment variables needed for wine commands
func (p *Prefix) Env() []string {
	env := []string{"WINEPREFIX=" + p.path}
	if p.display != "" {
		env = append(env, "DISPLAY="+p.display)
	}
	if p.log.GetLevel() > zerolog.DebugLevel {
		env = append(env, "WINEDEBUG=-all")
	}
	// Inherit WINEARCH from environment if set (required for 16-bit apps like Stars!)
	// WINEARCH=win32 creates a 32-bit prefix that can run 16-bit Windows 3.1 apps
	if winearch := os.Getenv("WINEARCH"); winearch != "" {
		env = append(env, "WINEARCH="+winearch)
	}
	return env
}

// EnsurePrefix ensures the wine prefix exists, creating it if necessary
func (p *Prefix) EnsurePrefix() error {
	info, err := os.Stat(p.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			dir := filepath.Dir(p.path)
			if err := os.MkdirAll(dir, 0770); err != nil { //nolint:gosec // Wine prefix requires group write access for Stars! executable
				p.log.Err(err).Msg("failed to create containing dir for wineprefix")
				return err
			}
			p.log.Info().Msg("initializing wineprefix... this can take some time")
			return p.createPrefix()
		}
		p.log.Err(err).Str("wineprefix", p.path).Msg("failed to stat wineprefix")
		return err
	}
	if info.IsDir() {
		return nil
	}
	return errors.New("wineprefix does not appear to be a directory")
}

// createPrefix initializes a new wine prefix
func (p *Prefix) createPrefix() error {
	c := cmd.NewCmd(winebootBin, winebootInit)
	c.Env = append(c.Env, p.Env()...)
	stdOut, stdErr, err := p.RunCmdWithTimeout(c)
	// Clean up any lingering wine processes after wineboot completes
	p.KillWineserver()
	if err != nil {
		msg := strings.Join(stdOut, "") + strings.Join(stdErr, "")
		p.log.Err(err).Msg(msg)
		return err
	}
	p.log.Debug().
		Str("out", strings.Join(stdOut, "\n")).
		Str("err", strings.Join(stdErr, "\n")).
		Msg("wineprefix created")
	return nil
}

// DevicesDir returns the path to the dosdevices directory in the wine prefix
func (p *Prefix) DevicesDir() string {
	return filepath.Join(p.path, dosDevicesDir)
}

// WindowsDir returns the path to the windows directory in the wine prefix
func (p *Prefix) WindowsDir() string {
	return filepath.Join(p.path, "drive_c", "windows")
}

// EnsureDriveLetter ensures a drive letter symlink exists pointing to the target directory
func (p *Prefix) EnsureDriveLetter(letter, targetDir string) error {
	devicesDir := p.DevicesDir()
	linkPath := filepath.Join(devicesDir, letter)

	fInfo, err := os.Lstat(linkPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.Symlink(targetDir, linkPath)
		}
		return err
	}

	m := fInfo.Mode()
	if m.IsDir() {
		return errors.New(letter + " should be a symlink, not a directory")
	}
	if m&fs.ModeSymlink != 0 {
		// Already a symlink, check if it points to the right place
		target, err := os.Readlink(linkPath)
		if err != nil {
			return err
		}
		if target != targetDir {
			p.log.Warn().
				Str("letter", letter).
				Str("current", target).
				Str("expected", targetDir).
				Msg("drive letter points to different directory, updating symlink")
			// Remove old symlink and create new one pointing to correct directory
			if err := os.Remove(linkPath); err != nil {
				return err
			}
			return os.Symlink(targetDir, linkPath)
		}
		return nil
	}
	return errors.New(letter + " should be a symlink, not a normal file")
}

// RunCommand runs a wine command with the prefix environment
func (p *Prefix) RunCommand(name string, args ...string) (stdout, stderr []string, err error) {
	c := cmd.NewCmd(name, args...)
	c.Env = append(c.Env, p.Env()...)
	return p.RunCmdWithTimeout(c)
}

// RunWine runs a program through wine with the prefix environment
func (p *Prefix) RunWine(program string, args ...string) (stdout, stderr []string, err error) {
	allArgs := append([]string{program}, args...)
	return p.RunCommand(wineBin, allArgs...)
}

// KillWineserver terminates all wine processes for this prefix using wineserver -k,
// then waits for them to fully exit using wineserver -w.
// This should be called after wine operations to clean up any lingering processes.
// It's safe to call even if no wine processes are running.
func (p *Prefix) KillWineserver() {
	// First, send kill signal to all wine processes
	killCmd := cmd.NewCmd(wineserverBin, wineserverKill)
	killCmd.Env = append(killCmd.Env, p.Env()...)
	statusChan := killCmd.Start()
	timeout := time.After(5 * time.Second)

	select {
	case status := <-statusChan:
		if status.Error != nil {
			p.log.Debug().Err(status.Error).Int("exit", status.Exit).Msg("wineserver -k completed with error (may be normal if no server running)")
		} else {
			p.log.Debug().Int("exit", status.Exit).Msg("wineserver -k completed")
		}
	case <-timeout:
		p.log.Warn().Msg("wineserver -k timed out")
		_ = killCmd.Stop()
		return
	}

	// Then wait for all wine processes to actually exit
	waitCmd := cmd.NewCmd(wineserverBin, wineserverWait)
	waitCmd.Env = append(waitCmd.Env, p.Env()...)
	statusChan = waitCmd.Start()
	timeout = time.After(10 * time.Second)

	select {
	case status := <-statusChan:
		if status.Error != nil {
			p.log.Debug().Err(status.Error).Int("exit", status.Exit).Msg("wineserver -w completed with error")
		} else {
			p.log.Debug().Int("exit", status.Exit).Msg("wineserver -w completed - all wine processes exited")
		}
	case <-timeout:
		p.log.Warn().Msg("wineserver -w timed out waiting for processes to exit")
		_ = waitCmd.Stop()
	}
}

// RunCmdWithTimeout runs a command with timeout
func (p *Prefix) RunCmdWithTimeout(c *cmd.Cmd) (stdout, stderr []string, err error) {
	p.log.Debug().
		Str("process", c.Name).
		Str("args", strings.Join(c.Args, " ")).
		Str("env", strings.Join(c.Env, " ")).
		Str("timeout", p.timeout.String()).
		Msg("starting process with timeout")

	statusChan := c.Start()
	timeout := time.After(p.timeout)

	var status cmd.Status
	select {
	case status = <-statusChan:
		p.log.Debug().Msg("process success")
	case <-timeout:
		p.log.Debug().Msg("process timed-out, killing")
		if err := c.Stop(); err != nil {
			p.log.Err(err).Msg("error killing timed-out process")
			return nil, nil, err
		}
		return nil, nil, errors.New("command timed out")
	}

	if status.Error != nil || status.Exit > 0 {
		return status.Stdout, status.Stderr, status.Error
	}
	return status.Stdout, status.Stderr, nil
}

// expand expands ~ and returns absolute path
func expand(input string) (string, error) {
	expanded, err := homedir.Expand(input)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}
