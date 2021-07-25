package main

import (
	"context"
	"os"
	"path"

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

	projRoot := "../../"
	projRoot = path.Join(wd, projRoot)

	buildOpts := esbuild.BuildOptions{
		Bundle: true,

		Format: esbuild.FormatIIFE,

		LogLevel:      esbuild.LogLevelDebug,
		EntryPoints:   []string{"src/index.tsx"},
		Tsconfig:      "tsconfig.json",
		AbsWorkingDir: projRoot,
		Sourcemap:     esbuild.SourceMapLinked,
	}
	serveOpts := esbuild.ServeOptions{
		Servedir: "src",
	}

	res, err := esbuild.Serve(serveOpts, buildOpts)
	if err != nil {
		return err
	}
	le.Infof("listening on %s port %d", res.Host, res.Port)

	return res.Wait()
}
