package npm

import (
	"context"
	"os"
	oexec "os/exec"
	"path/filepath"

	"github.com/aperturerobotics/util/autobun"
	"github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// BunTool installs the npm package pkg and returns a command that runs its
// executable bin with args.
//
// pkg is the package name with a pinned version, such as @electron/asar@4.3.0.
// The package installs once into stateDir/bun-tools/<bin> through
// EnsureBunAdd, which gives the directory its own download cache and install
// lock. Concurrent builds therefore share one install instead of racing on
// Bun's global cache. The command runs in the tool directory, so path
// arguments must be absolute.
func BunTool(ctx context.Context, le *logrus.Entry, stateDir, pkg, bin string, args ...string) (*oexec.Cmd, error) {
	toolDir := filepath.Join(stateDir, "bun-tools", bin)
	if err := EnsureBunAdd(ctx, le, stateDir, toolDir, pkg); err != nil {
		return nil, err
	}
	bunPath, err := ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}

	cmd := exec.NewCmd(ctx, bunPath, append([]string{"run", bin}, args...)...)
	cmd.Dir = toolDir
	return cmd, nil
}

// BunInstall runs "bun install" with the given arguments.
func BunInstall(ctx context.Context, le *logrus.Entry, stateDir string, installArgs ...string) (*oexec.Cmd, error) {
	bunPath, err := ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}

	args := append([]string{"install"}, bunMinimumReleaseAgeArg()...)
	args = append(args, installArgs...)
	return exec.NewCmd(ctx, bunPath, args...), nil
}

// BunAdd runs "bun add" to add a package.
func BunAdd(ctx context.Context, le *logrus.Entry, stateDir string, addArgs ...string) (*oexec.Cmd, error) {
	bunPath, err := ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, err
	}

	args := append([]string{"add"}, bunMinimumReleaseAgeArg()...)
	args = append(args, addArgs...)
	return exec.NewCmd(ctx, bunPath, args...), nil
}

func bunMinimumReleaseAgeArg() []string {
	minAge := os.Getenv("BLDR_BUN_MINIMUM_RELEASE_AGE")
	if minAge == "" {
		minAge = "0"
	}
	return []string{"--minimum-release-age=" + minAge}
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
