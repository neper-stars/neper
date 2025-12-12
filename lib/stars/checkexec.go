package stars

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

func CheckFileExecutable(file string, usePath bool) error {
	absoluteFile := file
	if usePath {
		var err error
		absoluteFile, err = exec.LookPath(file)
		if err != nil {
			return err
		}
	}

	fileInfo, err := os.Stat(absoluteFile)
	if err != nil {
		return err
	}

	m := fileInfo.Mode()
	if !m.IsRegular() && uint32(m&fs.ModeSymlink) != 0 {
		return errors.New("File " + absoluteFile + " is not a normal file or symlink.")
	}

	if uint32(m&0111) == 0 {
		return errors.New("File " + absoluteFile + " is not executable.")
	}

	if unix.Access(absoluteFile, unix.X_OK) != nil {
		return errors.New("File " + absoluteFile + " cannot be executed by this user.")
	}

	return nil
}
