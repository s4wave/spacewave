package main

import (
	"bytes"
	"os"
	"os/exec"

	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// Equivalent of:
// esbuild --tree-shaking=true --bundle --format=esm --platform=browser plugin-quickjs.ts --outfile=plugin-quickjs.esb.js
// followed by:
// rolldown plugin-quickjs.esb.js --file plugin-quickjs.esm.js --format esm
// Note: Rolldown tree-shakes the esbuild output while keeping the code readable.

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build banner.js into an IIFE that applies Symbol polyfills and sets
	// Event, EventTarget, CustomEvent on globalThis. This runs before any
	// bundled module code, ensuring classes like `class Foo extends Event`
	// work at import time.
	bannerResult := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints:  []string{"./quickjs/banner.ts"},
		Bundle:       true,
		Format:       esbuild_api.FormatIIFE,
		Platform:     esbuild_api.PlatformBrowser,
		Write:        false,
		MinifySyntax: true,
	})

	if len(bannerResult.Errors) > 0 {
		le.WithField("errors", bannerResult.Errors).Fatal("esbuild banner build failed")
		return
	}

	banner := string(bannerResult.OutputFiles[0].Contents)

	result := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{"./plugin-quickjs.ts"},
		Outfile:     "./plugin-quickjs.esb.js",
		Bundle:      true,
		TreeShaking: esbuild_api.TreeShakingTrue,
		Format:      esbuild_api.FormatESModule,
		Platform:    esbuild_api.PlatformBrowser,
		Write:       true,
		LogLevel:    esbuild_api.LogLevelInfo,
		// Use banner to inject Event/EventTarget polyfills at the very top.
		// This ensures classes extending Event work before any module code runs.
		Banner: map[string]string{
			"js": banner,
		},
	})

	if len(result.Errors) > 0 {
		le.WithField("errors", result.Errors).Fatal("esbuild failed with errors")
		return
	}

	if len(result.Warnings) > 0 {
		le.WithField("warnings", result.Warnings).Warn("esbuild completed with warnings")
	}

	le.Info("esbuild completed successfully")

	// Run rolldown to tree-shake the output.
	le.Info("running rolldown to tree-shake output")
	rolldownCmd := exec.Command(
		"bunx",
		"rolldown",
		"plugin-quickjs.esb.js",
		"--file", "plugin-quickjs.esm.js",
		"--format", "esm",
	)

	if err := rolldownCmd.Run(); err != nil {
		le.WithError(err).Fatal("rolldown failed")
		return
	}

	le.Info("rolldown completed successfully")

	// Add eslint-disable comment to the top of the generated file
	le.Info("adding eslint-disable comment to generated file")
	content, err := os.ReadFile("plugin-quickjs.esm.js")
	if err != nil {
		le.WithError(err).Fatal("failed to read generated file")
		return
	}

	eslintDisable := []byte("/* eslint-disable */\n")
	newContent := append(eslintDisable, trimTrailingLineWhitespace(content)...)
	// #nosec G703 -- writes a fixed generated filename in the current generator directory.
	if err := os.WriteFile("plugin-quickjs.esm.js", newContent, 0o644); err != nil {
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

func trimTrailingLineWhitespace(data []byte) []byte {
	lines := bytes.SplitAfter(data, []byte("\n"))
	out := make([]byte, 0, len(data))
	for _, line := range lines {
		hasNewline := len(line) > 0 && line[len(line)-1] == '\n'
		if hasNewline {
			line = line[:len(line)-1]
		}
		for len(line) > 0 {
			last := line[len(line)-1]
			if last != ' ' && last != '\t' {
				break
			}
			line = line[:len(line)-1]
		}
		out = append(out, line...)
		if hasNewline {
			out = append(out, '\n')
		}
	}
	return out
}
