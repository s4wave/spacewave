//go:build !js

package bldr_web_bundler_vite

import (
	"context"
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
	outputRoot := filepath.Dir(outputPath)
	outputName := filepath.Base(outputPath)
	entrypointName := strings.TrimSuffix(outputName, filepath.Ext(outputName))
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
	if got := result.GetEntrypointOutputs()[entrypointName]; got != outputName {
		return result, errors.Errorf("Vite service output is %q, expected %q", got, outputName)
	}
	return result, nil
}
