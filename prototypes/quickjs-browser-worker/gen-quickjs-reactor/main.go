package main

import (
	"os"
	"path/filepath"

	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// Bundles quickjs-wasi-reactor npm package into a single ES module.
// Output: quickjs-wasi-reactor.esm.js

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Find bldr root
	cwd, err := os.Getwd()
	if err != nil {
		le.WithError(err).Fatal("failed to get cwd")
		return
	}
	bldrRoot := filepath.Join(cwd, "../..")
	entryPoint := filepath.Join(bldrRoot, "node_modules/quickjs-wasi-reactor/dist/index.js")
	outfile := filepath.Join(cwd, "quickjs-wasi-reactor.esm.js")

	le.WithFields(logrus.Fields{
		"entryPoint": entryPoint,
		"outfile":    outfile,
	}).Info("bundling quickjs-wasi-reactor")

	result := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{entryPoint},
		Outfile:     outfile,
		Bundle:      true,
		TreeShaking: esbuild_api.TreeShakingTrue,
		Format:      esbuild_api.FormatESModule,
		Platform:    esbuild_api.PlatformBrowser,
		Write:       true,
		LogLevel:    esbuild_api.LogLevelInfo,
	})

	if len(result.Errors) > 0 {
		le.WithField("errors", result.Errors).Fatal("esbuild failed with errors")
		return
	}

	if len(result.Warnings) > 0 {
		le.WithField("warnings", result.Warnings).Warn("esbuild completed with warnings")
	}

	le.Info("esbuild completed successfully")

	// Add eslint-disable comment to the top of the generated file
	le.Info("adding eslint-disable comment to generated file")
	content, err := os.ReadFile(outfile)
	if err != nil {
		le.WithError(err).Fatal("failed to read generated file")
		return
	}

	eslintDisable := "/* eslint-disable */\n"
	newContent := eslintDisable + string(content)
	if err := os.WriteFile(outfile, []byte(newContent), 0o644); err != nil {
		le.WithError(err).Fatal("failed to write eslint-disable comment")
		return
	}

	le.Info("quickjs-wasi-reactor bundled successfully")
}
