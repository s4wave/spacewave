package runner

import (
	"io"
	"os"
	"time"

	"github.com/aperturerobotics/cli"
)

// ClientFlags builds command flags that select the client session and transport.
type ClientFlags func(sessionIdx *uint) []cli.Flag

// Config carries the injected transport and output sinks for shared CLI commands.
type Config struct {
	ClientFactory       ClientFactory
	ClientFlags         ClientFlags
	Stdout              io.Writer
	Now                 func() time.Time
	MountSessionTimeout func() (time.Duration, error)
}

func (c Config) defaults() Config {
	if c.ClientFlags == nil {
		c.ClientFlags = DefaultClientFlags
	}
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.MountSessionTimeout == nil {
		c.MountSessionTimeout = func() (time.Duration, error) {
			return defaultStatusMountSessionTimeout, nil
		}
	}
	return c
}

// DefaultClientFlags returns the browser-safe session flags for shared commands.
func DefaultClientFlags(sessionIdx *uint) []cli.Flag {
	return []cli.Flag{
		&cli.UintFlag{
			Name:        "session-index",
			Usage:       "session index to use",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: sessionIdx,
		},
	}
}
