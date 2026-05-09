//go:build !js

package spacewave_cli

import (
	"os"
	"runtime/trace"

	"github.com/pkg/errors"
)

func runWithRuntimeTrace(path string, cb func() error) error {
	if path == "" {
		return cb()
	}
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
