package devtool

import (
	"context"
	"errors"
)

// ExecuteWebProject starts the project as a web server.
func (a *DevtoolArgs) ExecuteWebProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	_ = repoRoot
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir)
	if err != nil {
		return err
	}
	defer b.Release()

	// execute the project controller
	// TODO

	_ = le
	return errors.New("TODO execute: web project")
}
