//go:build !js

package main

import (
	"context"
	"os"
	"path/filepath"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_vite "github.com/s4wave/spacewave/bldr/web/pkg/vite"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := run(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, le *logrus.Entry) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	rootDir := filepath.Join(wd, "../../")
	outDir := filepath.Join(wd, "out")
	workingDir := filepath.Join(wd, "working")

	refs := []*web_pkg.WebPkgRef{{
		WebPkgId:   "react",
		WebPkgRoot: filepath.Join(rootDir, "node_modules/react"),
		Imports:    []string{"index.js", "jsx-runtime.js"},
	}, {
		WebPkgId:   "react-dom",
		WebPkgRoot: filepath.Join(rootDir, "node_modules/react-dom"),
		Imports:    []string{"index.js", "client.js", "test-utils.js"},
	}, {
		WebPkgId:   "@aptre/bldr",
		WebPkgRoot: filepath.Join(rootDir, "web", "bldr"),
		Imports:    []string{"index.ts"},
	}, {
		WebPkgId:   "@aptre/bldr-react",
		WebPkgRoot: filepath.Join(rootDir, "web", "bldr-react"),
		Imports:    []string{"index.ts"},
	}}

	// Use the bldr dist sources as the distSourcePath.
	distSourcePath := filepath.Join(rootDir, ".bldr", "src")

	return web_pkg_vite.RunOneShot(ctx, le, distSourcePath, rootDir, workingDir, func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error {
		webPkgIds, srcPaths, importMapEntries, buildErr := web_pkg_vite.BuildWebPkgsVite(
			ctx,
			le,
			rootDir,
			refs,
			outDir,
			bldr_plugin.PluginWebPkgHttpPrefix,
			false,
			client,
			filepath.Join(workingDir, "cache"),
		)
		if buildErr != nil {
			return buildErr
		}
		le.Infof("web pkg ids: %v", webPkgIds)
		le.Infof("source paths: %d files", len(srcPaths))
		le.Infof("import map entries: %d", len(importMapEntries))
		for _, entry := range importMapEntries {
			le.Infof("  %s -> %s", entry.Specifier, entry.OutputPath)
		}
		return nil
	})
}
