package npm

import (
	"context"
	oexec "os/exec"

	"github.com/aperturerobotics/util/autobun"
	"github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// BunX runs "bunx" to execute a npm package without installing.
//
// pkg is the package name, optionally with the version:
//   - @electron/asar
//   - @electron/asar@3.2.3
func BunX(ctx context.Context, le *logrus.Entry, stateDir, pkg string, cmd ...string) (*oexec.Cmd, error) {
	bunPath, err := ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}

	args := []string{"x", pkg}
	args = append(args, cmd...)
	return exec.NewCmd(ctx, bunPath, args...), nil
}

// BunInstall runs "bun install" with the given arguments.
func BunInstall(ctx context.Context, le *logrus.Entry, stateDir string, installArgs ...string) (*oexec.Cmd, error) {
	bunPath, err := ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}

	args := []string{"install"}
	args = append(args, installArgs...)
	return exec.NewCmd(ctx, bunPath, args...), nil
}

// BunAdd runs "bun add" to add a package.
func BunAdd(ctx context.Context, le *logrus.Entry, stateDir string, addArgs ...string) (*oexec.Cmd, error) {
	bunPath, err := ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}

	args := []string{"add"}
	args = append(args, addArgs...)
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
