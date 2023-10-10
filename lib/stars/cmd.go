package stars

import (
	"fmt"
	"time"

	"strings"

	"github.com/go-cmd/cmd"
	"github.com/rs/zerolog"
)

// RunCMD runs the given cmd and returns its stdout, stderr and
// an eventual error
func RunCMD(c *cmd.Cmd) (stdout, stderr []string, err error) {
	statusChan := c.Start()
	status := <-statusChan
	if status.Error != nil || status.Exit > 0 {
		return status.Stdout, status.Stderr, fmt.Errorf("cannot execute command: %w", err)
	}
	stdout = status.Stdout
	stderr = status.Stderr
	return
}

func RunCMDTimeout(log *zerolog.Logger, c *cmd.Cmd, t time.Duration) (stdOut, stdErr []string, err error) {
	log.Debug().
		Str("process", c.Name).
		Str("args", strings.Join(c.Args, " ")).
		Str("env", strings.Join(c.Env, " ")).
		Str("timeout", t.String()).
		Msg("starting process with timeout")
	statusChan := c.Start()
	timeout := time.After(t)
	var status cmd.Status
	select {
	case status = <-statusChan:
		log.Debug().Msg("process success")
	case <-timeout:
		log.Debug().Msg("process timed-out, killing")
		if err := c.Stop(); err != nil {
			log.Err(err).Msg("error killing timed-out process")
			return nil, nil, err
		}
	}

	if status.Error != nil || status.Exit > 0 {
		return status.Stdout, status.Stderr, fmt.Errorf("could not execute command: %w", err)
	}
	stdOut = status.Stdout
	stdErr = status.Stderr
	return
}
