package main

import (
	"os"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// Bundles wasi-shim TypeScript into a single ES module.
// Output: wasi-shim.esm.js

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	result := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{"./index.ts"},
		Outfile:     "./wasi-shim.esm.js",
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
	content, err := os.ReadFile("wasi-shim.esm.js")
	if err != nil {
		le.WithError(err).Fatal("failed to read generated file")
		return
	}

	eslintDisable := "/* eslint-disable */\n"
	newContent := eslintDisable + string(content)
	// #nosec G703 -- writes a fixed generated filename in the current generator directory.
	if err := os.WriteFile("wasi-shim.esm.js", []byte(newContent), 0o644); err != nil {
		le.WithError(err).Fatal("failed to write eslint-disable comment")
		return
	}

	le.Info("wasi-shim bundled successfully")
}
