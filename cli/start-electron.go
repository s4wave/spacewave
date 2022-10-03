package cli

import (
	"context"
	"errors"
	"path"

	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/entrypoint/electron/bundle"
	esbuild "github.com/evanw/esbuild/pkg/api"
)

// ExecuteElectron starts the application as an electron app.
func (a *DevtoolArgs) ExecuteElectron(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	_ = repoRoot
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	dtBus, err := BuildDevtoolBus(ctx, le, stateDir)
	if err != nil {
		return err
	}
	defer dtBus.Release()

	webSrcDir := dtBus.GetWebSrcDir()
	entrypointDataDir := path.Join(stateDir, "entrypoint")
	entrypointDir := path.Join(entrypointDataDir, "electron")

	// run esbuild to compile the electron entrypoint
	le.Info("building electron entrypoint")
	entrypoint_electron_bundle.EsbuildLogLevel = esbuild.LogLevelError
	err = entrypoint_electron_bundle.BuildBrowserBundle(le, webSrcDir, entrypointDir, true)
	if err != nil {
		return err
	}

	// access the devtool world state
	worldState := dtBus.GetWorldState()
	_ = worldState

	return errors.New("TODO: launch electron")
}
