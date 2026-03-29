package bun

import (
	"context"
	oexec "os/exec"

	"github.com/aperturerobotics/util/autobun"
	"github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// BunExec builds a command to run a script with Bun.
// stateDir is the directory where bun will be downloaded if not found in PATH.
// If stateDir is empty, bun must be in the system PATH.
func BunExec(ctx context.Context, le *logrus.Entry, stateDir, filePath string, fileArgs ...string) (*oexec.Cmd, error) {
	bunPath, err := ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}

	args := []string{filePath}
	if len(fileArgs) != 0 {
		args = append(args, "--")
		args = append(args, fileArgs...)
	}
	return exec.NewCmd(ctx, bunPath, args...), nil
}

// ResolveBunPath resolves the path to the bun binary.
// If bun is in PATH, returns that path.
// If not, downloads bun to stateDir and returns that path.
// If stateDir is empty and bun is not in PATH, returns an error.
func ResolveBunPath(ctx context.Context, le *logrus.Entry, stateDir string) (string, error) {
	// If stateDir is empty, just use system PATH
	if stateDir == "" {
		return oexec.LookPath("bun")
	}

	// Use autobun to ensure bun is available
	return autobun.EnsureBun(ctx, le, stateDir, autobun.DefaultBunVersion)
}
