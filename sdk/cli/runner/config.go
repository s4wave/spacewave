package runner

import (
	"fmt"
	"io"
	"math"
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
			Action: func(_ *cli.Context, value uint) error {
				if value > math.MaxUint32 {
					return fmt.Errorf("session-index exceeds uint32 range: %d", value)
				}
				return nil
			},
		},
	}
}

func sessionIndex32(value uint) uint32 {
	if value > math.MaxUint32 {
		panic("session index must be validated before conversion")
	}
	return uint32(value) //nolint:gosec // the range check enforces the session-index API.
}
