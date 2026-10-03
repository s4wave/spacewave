//go:build !js

package cli_entrypoint

import (
	"os"
	"runtime/trace"

	"github.com/pkg/errors"
)

func runWithRuntimeTrace(path string, cb func() error) error {
	// Run the callback directly when runtime tracing is disabled.
	if path == "" {
		return cb()
	}

	// Capture the callback runtime trace in the requested file.
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "create runtime trace")
	}
	defer f.Close()
	if err := trace.Start(f); err != nil {
		return errors.Wrap(err, "start runtime trace")
	}
	defer trace.Stop()

	return cb()
}
