package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	web_pkg_esbuild "github.com/aperturerobotics/bldr/web/pkg/esbuild"
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

	refs := []*web_pkg_esbuild.WebPkgRef{{
		WebPkgID:   "react",
		WebPkgRoot: filepath.Join(rootDir, "node_modules/react"),
		Imports:    []string{"index.js", "jsx-runtime.js"},
	}, {
		WebPkgID:   "react-dom",
		WebPkgRoot: filepath.Join(rootDir, "node_modules/react-dom"),
		Imports:    []string{"index.js", "client.js"},
	}}

	webPkgIds, srcPaths, err := web_pkg_esbuild.BuildWebPkgsEsbuild(
		ctx,
		le,
		rootDir,
		refs,
		outDir,
		bldr_plugin.PluginWebPkgHttpPrefix,
		false,
	)
	if err != nil {
		return err
	}
	fmt.Printf("web pkg ids: %v\n", webPkgIds)
	fmt.Printf("source paths: %v\n", srcPaths)
	return nil
}
