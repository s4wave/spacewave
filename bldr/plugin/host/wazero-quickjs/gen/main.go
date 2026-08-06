package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	"github.com/sirupsen/logrus"
)

const (
	generatedFilename    = "plugin-quickjs.esm.js"
	intermediateFilename = "plugin-quickjs.esb.js"
)

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	if err := generate(context.Background(), logrus.NewEntry(log)); err != nil {
		log.WithError(err).Fatal("generate QuickJS plugin runtime")
	}
}

func generate(ctx context.Context, le *logrus.Entry) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	bldrRoot := filepath.Clean(filepath.Join(workingDir, "../../.."))
	stateDir, err := os.MkdirTemp("", "bldr-quickjs-generator-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stateDir)

	if err := os.Remove(filepath.Join(workingDir, intermediateFilename)); err != nil && !os.IsNotExist(err) {
		return err
	}
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		stateDir,
		bldrRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   workingDir,
			SourceRoot:   bldrRoot,
			OutputRoot:   workingDir,
			BldrDistRoot: bldrRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      "plugin-quickjs",
				InputPath: filepath.Join(workingDir, "plugin-quickjs.ts"),
			}},
			Format:         "es",
			Platform:       "browser",
			EntryFileNames: generatedFilename,
			ChunkFileNames: "[name]-[hash].mjs",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      "none",
			Minify:         true,
			TreeShaking:    true,
			Banner:         "/* eslint-disable */",
			Inject:         []string{filepath.Join(workingDir, "quickjs/banner.ts")},
		},
	)
	if err != nil {
		return err
	}
	if output := result.GetEntrypointOutputs()["plugin-quickjs"]; output != generatedFilename {
		return fmt.Errorf("QuickJS generator output = %q, want %q", output, generatedFilename)
	}
	return nil
}
