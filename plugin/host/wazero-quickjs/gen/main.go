package main

import (
	"os"
	"os/exec"

	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// Equivalent of:
// esbuild --tree-shaking=true --bundle --format=esm --platform=browser plugin-quickjs.ts --outfile=plugin-quickjs.esb.js
// followed by:
// rollup plugin-quickjs.esb.js --file plugin-quickjs.esm.js --format es --plugin @rollup/plugin-terser
// Note: We use rollup with terser for minification while keeping the code readable

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	result := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{"./plugin-quickjs.ts"},
		Outfile:     "./plugin-quickjs.esb.js",
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

	// Run rollup to minify the output
	le.Info("running rollup to tree-shake output")
	rollupCmd := exec.Command(
		"../../../node_modules/.bin/rollup",
		"plugin-quickjs.esb.js",
		"--file", "plugin-quickjs.esm.js",
		"--format", "es",
		// "--plugin", "@rollup/plugin-terser",
	)

	if err := rollupCmd.Run(); err != nil {
		le.WithError(err).Fatal("rollup failed")
		return
	}

	le.Info("rollup completed successfully")

	// Add eslint-disable comment to the top of the generated file
	le.Info("adding eslint-disable comment to generated file")
	content, err := os.ReadFile("plugin-quickjs.esm.js")
	if err != nil {
		le.WithError(err).Fatal("failed to read generated file")
		return
	}

	eslintDisable := "/* eslint-disable */\n"
	newContent := eslintDisable + string(content)
	if err := os.WriteFile("plugin-quickjs.esm.js", []byte(newContent), 0o644); err != nil {
		le.WithError(err).Fatal("failed to write eslint-disable comment")
		return
	}

	// Clean up intermediate file
	if err := os.Remove("plugin-quickjs.esb.js"); err != nil {
		le.WithError(err).Warn("failed to remove intermediate file")
	} else {
		le.Info("cleaned up intermediate file")
	}
}
