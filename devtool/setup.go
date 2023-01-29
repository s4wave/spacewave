package devtool

import "context"

// ExecuteSetup executes the Setup CLI command.
func (a *DevtoolArgs) ExecuteSetup(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	_, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("initializing state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir, false)
	if err != nil {
		return err
	}
	defer b.Release()

	return b.SyncWebSources(a.BldrVersion, a.BldrVersionSum)
}
