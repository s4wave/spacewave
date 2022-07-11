package main

import (
	"context"
	"os"

	browser "github.com/aperturerobotics/bldr/entrypoint/browser/bundle"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var repoRoot string
var outFile string

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// default repoRoot to initial workdir
	repoRoot, _ = os.Getwd()

	app := cli.NewApp()
	app.Name = "bundle"
	app.Usage = "basic prototype of bundling a component w/o containers"
	app.HideVersion = true
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "repo-root",
			Usage:       "path to the root of the repo containing tsconfig.json",
			Destination: &repoRoot,
			Value:       repoRoot,
		},
		cli.StringFlag{
			Name:        "out, o",
			Usage:       "path to the output. defaults to stdout",
			Destination: &outFile,
			Value:       outFile,
		},
	}
	app.Action = func(c *cli.Context) error {
		args := c.Args()
		if len(args) == 0 {
			return errors.New("usage: ./bundle Component.tsx")
		}
		return runBundlePrototype(ctx, le, args)
	}

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

// runBundlePrototype runs the bundling prototype.
func runBundlePrototype(ctx context.Context, le *logrus.Entry, entrypoints []string) error {
	minify := true
	buildOpts := BundleComponentBuildOpts(repoRoot, minify)
	buildOpts.EntryPoints = entrypoints
	buildOpts.Outfile = outFile
	buildOpts.Write = outFile != ""
	res := esbuild.Build(buildOpts)
	if err := browser.EsbuildErrorsToError(res); err != nil {
		return err
	}
	if len(res.OutputFiles) != 1 {
		return errors.Errorf("expected 1 output file but got %d", len(res.OutputFiles))
	}
	_, _ = os.Stdout.WriteString(string(res.OutputFiles[0].Contents) + "\n")
	return nil
}
