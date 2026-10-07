//go:build !js

// Command prepare builds the shared-daemon fixture with the production Electron shell.
package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	electron_bundle "github.com/s4wave/spacewave/bldr/web/entrypoint/electron/bundle"
	"github.com/sirupsen/logrus"
)

// main builds the fixture in the requested directory.
func main() {
	// Require an explicit output directory.
	le := logrus.NewEntry(logrus.New())
	if len(os.Args) != 2 {
		le.Fatal("usage: prepare <output-directory>")
	}
	if err := prepare(os.Args[1]); err != nil {
		le.Fatal(err)
	}
}

// prepare compiles the shell, renderer, and worker into one Electron app.
func prepare(destination string) error {
	// Resolve the source and output directories.
	root, err := filepath.Abs(".")
	if err != nil {
		return err
	}

	// Resolve the destination independently from the source checkout.
	out, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	// Keep bundler state beside the prepared app.
	bldrRoot := filepath.Join(root, "bldr")
	stateDir := filepath.Join(out, "build-state")
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Compile the production Electron main process and preload bridge.
	if err := electron_bundle.BuildMainBundle(ctx, le, stateDir, bldrRoot, out, false, true); err != nil {
		return errors.Wrap(err, "build Electron main")
	}
	if err := electron_bundle.BuildPreloadBundle(ctx, le, stateDir, bldrRoot, out, false, true); err != nil {
		return errors.Wrap(err, "build Electron preload")
	}

	// Bundle the fixture renderer and worker with production Resource clients.
	_, err = rolldown.Build(ctx, le, stateDir, bldrRoot, &rolldown.BuildRequest{
		WorkingDir:   root,
		SourceRoot:   root,
		OutputRoot:   out,
		BldrDistRoot: bldrRoot,
		Entrypoints: []*rolldown.Entrypoint{
			{Name: "renderer", InputPath: filepath.Join(root, "e2e/shareddaemon/renderer.ts")},
			{Name: "worker", InputPath: filepath.Join(root, "e2e/shareddaemon/worker.ts")},
		},
		Format:         "es",
		Platform:       "browser",
		Target:         "es2024",
		EntryFileNames: "[name].mjs",
		ChunkFileNames: "[name]-[hash].mjs",
		AssetFileNames: "[name]-[hash][extname]",
		Sourcemap:      "none",
		TreeShaking:    true,
		CodeSplitting:  true,
		Defines:        electron_bundle.ElectronDefine(true),
	})
	if err != nil {
		return errors.Wrap(err, "build fixture renderer and worker")
	}

	// Publish the Electron app entrypoint and the visible fixture document.
	if err := os.WriteFile(filepath.Join(out, "package.json"), []byte(`{"name":"shared-daemon-fixture","version":"0.0.1","main":"index.mjs"}`), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "index.html"), []byte(`<!doctype html><html><head><meta charset="utf-8"><title>Shared daemon fixture</title></head><body><pre id="identity">Connecting</pre><script type="module" src="./renderer.mjs"></script></body></html>`), 0o644)
}
