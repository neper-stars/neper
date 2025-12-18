package wine

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

// CheckExecutable checks if a file exists and is executable.
// If inPath is true, it searches for the file in PATH.
func CheckExecutable(filename string, inPath bool) error {
	absoluteFile := filename
	if inPath {
		path, err := exec.LookPath(filename)
		if err != nil {
			return err
		}
		absoluteFile = path
	}

	info, err := os.Stat(absoluteFile)
	if err != nil {
		return err
	}

	m := info.Mode()
	if !m.IsRegular() && m&fs.ModeSymlink == 0 {
		return errors.New(absoluteFile + " is not a regular file or symlink")
	}

	if m&0111 == 0 {
		return errors.New(absoluteFile + " is not executable")
	}

	if unix.Access(absoluteFile, unix.X_OK) != nil {
		return errors.New(absoluteFile + " cannot be executed by this user")
	}

	return nil
}

// CheckWine checks if wine is available in PATH
func CheckWine() error {
	return CheckExecutable(wineBin, true)
}

// CheckXvfb checks if Xvfb is available in PATH
func CheckXvfb() error {
	return CheckExecutable("Xvfb", true)
}
