package main

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/bldr/entrypoint/browser/bundle"
	esbuild "github.com/evanw/esbuild/pkg/api"
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
	repoRoot := path.Join(wd, "../../")

	targetDir := path.Join(repoRoot, "target/browser")
	buildDir := path.Join(targetDir, "build")
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		err = os.RemoveAll(buildDir)
		if err != nil {
			return err
		}
	}

	minify := false
	err = entrypoint_browser_bundle.BuildBrowserBundle(le, repoRoot, buildDir, minify)
	if err != nil {
		return err
	}

	// Build & serve the entrypoint TypeScript with hot-reloading.
	buildOpts := entrypoint_browser_bundle.BrowserEntrypointBuildOpts(repoRoot, minify)
	buildOpts.Bundle = true
	buildOpts.Format = esbuild.FormatESModule

	serveOpts := esbuild.ServeOptions{
		Servedir: buildDir,
	}
	res, err := esbuild.Serve(serveOpts, buildOpts)
	if err != nil {
		return err
	}
	le.Infof("listening on %s port %d", res.Host, res.Port)

	return res.Wait()
}
