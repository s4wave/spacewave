// Prototype of building the Web entrypoint bundle.
package main

import (
	"context"
	"os"
	"path"

	"github.com/sirupsen/logrus"
	// esbuild "github.com/evanw/esbuild/pkg/api"
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
	outDir := path.Join(projRoot, "build", "web")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	// Step: use esbuild to compile the entrypoint tsx.
	le.Info("bundling web entrypoint")

	return err
}
