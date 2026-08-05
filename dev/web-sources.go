//go:build !js

package dev

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	bldr_devtool "github.com/s4wave/spacewave/bldr/devtool"
)

// spacewaveModulePath is the Go module that owns the @s4wave/web web pkg.
const spacewaveModulePath = "github.com/s4wave/spacewave"

// webPkgScopeDir and webPkgName form the materialized web pkg directory that the
// bldr resolver maps "@s4wave/web" to: "<repoRoot>/.s4wave/web".
const (
	webPkgScopeDir = ".s4wave"
	webPkgName     = "web"
)

// ensureWebSources materializes the spacewave web sources for a downstream app.
// go mod vendor strips spacewave's pure-TS web dirs from the vendor tree, so the
// bldr build cannot resolve @s4wave/web from .bldr/src/vendor. The full source
// lives in the spacewave module dir (module cache, or a local replace target);
// link it to .s4wave/web, where the bldr web pkg resolver looks for a
// materialized "@scope/name". Best effort: on failure the existing resolver
// stages run and surface the original build error.
func ensureWebSources(ctx context.Context, args *bldr_devtool.DevtoolArgs) {
	// Resolve the downstream repository root.
	le := args.Logger
	repoRoot, err := args.FindRepoRoot()
	if err != nil {
		le.WithError(err).Debug("ensure web sources: resolve repo root")
		return
	}

	// Resolve the spacewave module directory from the module graph.
	moduleDir, err := spacewaveModuleDir(ctx, repoRoot)
	if err != nil {
		le.WithError(err).Debug("ensure web sources: resolve spacewave module dir")
		return
	}

	// Require the module's web source directory before linking it.
	source := filepath.Join(moduleDir, webPkgName)
	if info, statErr := os.Stat(source); statErr != nil || !info.IsDir() {
		le.WithError(statErr).Debugf("ensure web sources: missing %s", source)
		return
	}

	// Create the scope directory used for the materialized web package.
	scope := filepath.Join(repoRoot, webPkgScopeDir)
	if err := os.MkdirAll(scope, 0o755); err != nil {
		le.WithError(err).Debug("ensure web sources: create scope dir")
		return
	}

	// Reuse an existing link when it already targets the module source.
	link := filepath.Join(scope, webPkgName)
	if current, readErr := os.Readlink(link); readErr == nil && current == source {
		return
	}

	// Remove a stale link before creating the current source link.
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		le.WithError(err).Debug("ensure web sources: clear stale link")
		return
	}
	if err := os.Symlink(source, link); err != nil {
		le.WithError(err).Debug("ensure web sources: link web pkg")
		return
	}
	le.Debugf("linked %s -> %s", link, source)
}

// spacewaveModuleDir returns the on-disk source directory for the spacewave
// module as resolved from repoRoot's module graph, downloading it if needed.
func spacewaveModuleDir(ctx context.Context, repoRoot string) (string, error) {
	// Ask Go to resolve the spacewave module directory.
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-mod=mod", "-f", "{{.Dir}}", spacewaveModulePath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(err, "go list spacewave module")
	}

	// Require a non-empty module directory for source linking.
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", errors.New("spacewave module dir is empty")
	}
	return dir, nil
}
