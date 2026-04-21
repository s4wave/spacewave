//go:build !js

package devtool

import "context"

// ExecuteSetup executes the Setup CLI command.
func (a *DevtoolArgs) ExecuteSetup(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("initializing state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, false)
	if err != nil {
		return err
	}
	defer b.Release()

	return b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath)
}
