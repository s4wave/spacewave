package main

import (
	esbuild_api "github.com/evanw/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// Equivalent of:
// esbuild --tree-shaking=true --bundle --format=esm --platform=browser plugin-quickjs.ts --outfile=plugin-quickjs.esm.js

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	result := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{"./plugin-quickjs.ts"},
		Outfile:     "./plugin-quickjs.esm.js",
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
}
