// Command sync-library compiles and bundles the standalone npm package.
package main

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	"github.com/sirupsen/logrus"
)

func main() {
	// Parse the output and compiler flags for the build.
	output := flag.String("output", "packages/spacewave/dist", "Directory for the compiled npm exports")
	skipCompile := flag.Bool("skip-compile", false, "Reuse the existing compiled engine for a TypeScript-only change")
	overrideDir := flag.String("override-dir", "", "GoScript runtime override directory for compiler development")
	flag.Parse()

	// Run the build and exit nonzero when it fails.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	if err := build(ctx, le, *output, *skipCompile, *overrideDir); err != nil {
		le.Error(err)
		os.Exit(1)
	}
}

// build uses the same compiler binding discovery and bundle owner as Bldr.
func build(ctx context.Context, le *logrus.Entry, output string, skipCompile bool, overrideDir string) error {
	// Resolve the repository root and the absolute output directory.
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}

	// Compile the GoScript engine unless the caller reuses it.
	working := filepath.Join(root, ".tmp", "sync-library")
	compiled := filepath.Join(working, "goscript")
	if !skipCompile {
		env := []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}
		bindings, err := gocompiler.GoScriptBindingRoots(ctx, root, env...)
		if err != nil {
			return err
		}
		var overrides []string
		if overrideDir != "" {
			overrides = []string{overrideDir}
		}
		if err := gocompiler.ExecGoScriptCompile(ctx, le, gocompiler.GoScriptCompileOptions{
			WorkDir: root, OutputPath: compiled,
			Packages:   []string{"./core/sync/node"},
			BuildFlags: []string{"-tags=goscript,skip_e2e,purego"},
			Env:        env, BindingRoots: bindings, OverrideDirs: overrides,
			AllDependencies: true, ProtobufTypeScriptBinding: true,
		}); err != nil {
			return err
		}
	}

	// Write the worker entrypoint into the working directory.
	entrypoint := filepath.Join(working, "engine-worker.ts")
	if err := os.WriteFile(entrypoint, []byte(workerEntrypoint), 0o644); err != nil {
		return err
	}

	// Bundle the node platform build with the GoScript engine.
	result, err := rolldown.Build(ctx, le, working, filepath.Join(root, "bldr"), &rolldown.BuildRequest{
		WorkingDir: working, SourceRoot: root, OutputRoot: output,
		BldrDistRoot: filepath.Join(root, "bldr"),
		Format:       "es", Platform: "node", Target: "es2024",
		EntryFileNames: "[name].mjs", ChunkFileNames: "chunks/[name]-[hash].mjs",
		AssetFileNames: "assets/[name]-[hash][extname]",
		CodeSplitting:  true, Sourcemap: "none", TreeShaking: true, Minify: true,
		CleanOutputDir: true,
		Defines:        map[string]string{"process.env.WS_NO_BUFFER_UTIL": "true", "process.env.WS_NO_UTF_8_VALIDATE": "true"},
		Entrypoints: []*rolldown.Entrypoint{
			{Name: "engine-worker", InputPath: entrypoint},
			{Name: "server", InputPath: filepath.Join(root, "packages/spacewave/server.ts")},
		},
		Goscript: &rolldown.GoScriptPolicy{OutputRoot: compiled},
	})
	if err != nil {
		return err
	}

	// Bundle the browser platform build for the public client entrypoints.
	client, err := rolldown.Build(ctx, le, working, filepath.Join(root, "bldr"), &rolldown.BuildRequest{
		WorkingDir: working, SourceRoot: root, OutputRoot: output,
		BldrDistRoot: filepath.Join(root, "bldr"),
		Format:       "es", Platform: "browser", Target: "es2024",
		EntryFileNames: "[name].mjs", ChunkFileNames: "chunks/[name]-[hash].mjs",
		AssetFileNames: "assets/[name]-[hash][extname]",
		CodeSplitting:  true, Sourcemap: "none", TreeShaking: true, Minify: true,
		External: []string{"react", "react/jsx-runtime"},
		Entrypoints: []*rolldown.Entrypoint{
			{Name: "index", InputPath: filepath.Join(root, "packages/spacewave/index.ts")},
			{Name: "react", InputPath: filepath.Join(root, "packages/spacewave/react.ts")},
		},
	})
	if err != nil {
		return err
	}

	// Write both build reports into the working directory.
	clientReport, err := client.MarshalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(working, "client-build-report.json"), clientReport, 0o644); err != nil {
		return err
	}
	report, err := result.MarshalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(working, "build-report.json"), report, 0o644); err != nil {
		return err
	}

	// Generate TypeScript declarations for the output.
	declarations := exec.CommandContext(ctx, "bun", "run", "scripts/sync-library/declarations.ts", output)
	declarations.Dir = root
	declarations.Stdout = os.Stdout
	declarations.Stderr = os.Stderr
	if err := declarations.Run(); err != nil {
		return err
	}

	// Generate license notices for the output.
	notices := exec.CommandContext(ctx, "bun", "run", "scripts/sync-library/notices.ts", output)
	notices.Dir = root
	notices.Stdout = os.Stdout
	notices.Stderr = os.Stderr
	return notices.Run()
}

// workerEntrypoint adapts generated Go values to the typed Worker host contract.
// Its relative host import is rooted at the build's .tmp/sync-library directory.
const workerEntrypoint = `import { ValueOf } from '@goscript/syscall/js/index.js'
import { Open } from '@goscript/github.com/s4wave/spacewave/core/sync/node/runtime.gs.js'
import { runWorker } from '../../core/sync/node/worker.js'

runWorker(async (directory, openSQLPort) => {
  const [runtime, error] = await Open(directory, ValueOf(openSQLPort))
  if (error) throw new Error(await error.Error())
  if (!runtime) throw new Error('Sync engine returned no runtime')
  return {
    async accept(port) {
      const error = await runtime.Accept(ValueOf(port))
      if (error) throw new Error(await error.Error())
    },
    async close() {
      const error = await runtime.Close()
      if (error) throw new Error(await error.Error())
    },
  }
})
`
