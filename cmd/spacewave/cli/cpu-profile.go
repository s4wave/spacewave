//go:build !js

package spacewave_cli

import (
	stderrors "errors"
	"os"
	"runtime/pprof"

	"github.com/pkg/errors"
)

// startCPUProfile writes a Go CPU profile of this process to a private file
// at path, from the call until stop, which ends the profile and closes the
// file. An empty path profiles nothing. A one-shot command uses it, since
// the daemon's profile RPC reaches only a running daemon.
func startCPUProfile(path string) (stop func() error, err error) {
	// Leave an unprofiled run with nothing to stop.
	if path == "" {
		return func() error { return nil }, nil
	}

	// Sample into the file until the caller stops the profile.
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- the operator names the profile's file.
	if err != nil {
		return nil, errors.Wrap(err, "create cpu profile")
	}
	if err := pprof.StartCPUProfile(out); err != nil {
		return nil, stderrors.Join(errors.Wrap(err, "start cpu profile"), errors.Wrap(out.Close(), "close cpu profile"))
	}
	return func() error {
		pprof.StopCPUProfile()
		return errors.Wrap(out.Close(), "close cpu profile")
	}, nil
}
