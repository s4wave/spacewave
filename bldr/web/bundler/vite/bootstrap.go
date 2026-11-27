//go:build !js

package bldr_web_bundler_vite

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	"github.com/sirupsen/logrus"
)

// BuildServiceScript builds the Node.js host for one Vite service process.
func BuildServiceScript(
	ctx context.Context,
	le *logrus.Entry,
	stateDir,
	sourceRoot,
	bldrDistRoot,
	outputPath string,
) (*bldr_web_bundler_rolldown.BuildResult, error) {
	// Split the output path into its root, file name, and entrypoint name.
	outputRoot := filepath.Dir(outputPath)
	outputName := filepath.Base(outputPath)
	entrypointName := strings.TrimSuffix(outputName, filepath.Ext(outputName))

	// Build the Node host through the Rolldown bundler contract.
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		stateDir,
		bldrDistRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   outputRoot,
			SourceRoot:   sourceRoot,
			OutputRoot:   outputRoot,
			BldrDistRoot: bldrDistRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      entrypointName,
				InputPath: filepath.Join(bldrDistRoot, ResolveViteEntrypointPath(bldrDistRoot)),
			}},
			Format:           "es",
			Platform:         "node",
			Target:           "es2022",
			EntryFileNames:   outputName,
			ChunkFileNames:   "[name]-[hash].mjs",
			AssetFileNames:   "[name]-[hash][extname]",
			Sourcemap:        "external",
			TreeShaking:      true,
			Defines:          map[string]string{"BLDR_IS_NODE": "true", "NO_COLOR": "1"},
			External:         []string{"starpc", "vite"},
			ExternalPackages: true,
		},
	)
	if err != nil {
		return result, errors.Wrap(err, "build Vite service script")
	}

	// Require the build to report the requested entrypoint output name.
	if got := result.GetEntrypointOutputs()[entrypointName]; got != outputName {
		return result, errors.Errorf("Vite service output is %q, expected %q", got, outputName)
	}

	// Resolve the script's external packages from the pinned dist install.
	if err := linkServiceNodeModules(ctx, le, stateDir, bldrDistRoot, outputRoot); err != nil {
		return result, err
	}
	return result, nil
}

// linkServiceNodeModules points outputRoot/node_modules at the pinned Bldr
// dist install. The service script imports vite, starpc, and
// @aptre/protobuf-es-lite as external packages, and bun resolves them from the
// script's directory upward. A working directory outside any project (a
// temporary directory) has no node_modules above it, so bun would otherwise
// auto-install whatever version its global cache holds, which can predate the
// generated messages the script loads. A real node_modules directory already
// in outputRoot is left alone.
func linkServiceNodeModules(ctx context.Context, le *logrus.Entry, stateDir, bldrDistRoot, outputRoot string) error {
	// Locate the dist install that Rolldown already verified for this build.
	depsRoot, err := bldr_web_bundler_rolldown.EnsureDependencyRoot(ctx, le, stateDir, bldrDistRoot)
	if err != nil {
		return errors.Wrap(err, "locate Vite service dependencies")
	}
	target := filepath.Join(depsRoot, "node_modules")
	link := filepath.Join(outputRoot, "node_modules")

	// Keep a correct link and replace a stale one. Refuse to touch a real
	// directory.
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		if current, err := os.Readlink(link); err == nil && current == target {
			return nil
		}
		if err := os.Remove(link); err != nil {
			return errors.Wrap(err, "replace Vite service node_modules link")
		}
	}
	if err := os.Symlink(target, link); err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "link Vite service node_modules")
	}
	return nil
}
