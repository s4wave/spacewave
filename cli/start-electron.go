package cli

import (
	"context"
	"errors"
	"os"
	"path"
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

	// mount the entrypoint unixfs
	worldState := dtBus.GetWorldState()
	_ = worldState

	entrypointDataDir := path.Join(stateDir, "entry")
	entrypointSrcDir := path.Join(entrypointDataDir, "src", "electron")
	entrypointDir := path.Join(entrypointDataDir, "electron")

	// checkout the entrypoint sources to the path
	err = os.MkdirAll(entrypointDir, 0755)
	if err == nil {
		err = os.MkdirAll(entrypointSrcDir, 0755)
	}
	if err != nil {
		return err
	}

	return errors.New("TODO: checkout electron entrypoint")

	// construct sources iofs

	// unixfs_sync.Sync(ctx, , fsHandle *unixfs.FSHandle, deleteMode unixfs_sync.DeleteMode)

	// compile the entrypoint files from the sources

	// cleanup the sources dir
}
