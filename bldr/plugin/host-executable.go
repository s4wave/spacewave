package bldr_plugin

import (
	"os"

	"github.com/pkg/errors"
)

// HostExecutableEnv carries the plugin host's executable path to process
// plugins.
const HostExecutableEnv = "BLDR_PLUGIN_HOST_EXECUTABLE"

// startInfoEnv carries the start info to process plugins.
const startInfoEnv = "BLDR_PLUGIN_START_INFO"

// ErrHostExecutableUnknown is returned when a process plugin's host did not
// report its executable.
var ErrHostExecutableUnknown = errors.New("plugin host executable unknown")

// HostExecutable returns the path of the executable hosting this process.
//
// A process plugin reads it from HostExecutableEnv. Any other process is its
// own host and returns os.Executable.
func HostExecutable() (string, error) {
	if exe := os.Getenv(HostExecutableEnv); exe != "" {
		return exe, nil
	}
	if os.Getenv(startInfoEnv) != "" {
		return "", ErrHostExecutableUnknown
	}
	return os.Executable()
}
