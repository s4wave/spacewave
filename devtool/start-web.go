package devtool

import (
	"context"
	"errors"
	"os"
	"path"

	"github.com/sirupsen/logrus"
)

// ExecuteWeb starts the application as a web server.
func (a *DevtoolArgs) ExecuteWeb(ctx context.Context, le *logrus.Entry) error {
	// init repo root and storage directories
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

	// TODO
	// checkout the entrypoint UnixFS to the path
	webEntrypointDir := path.Join(stateDir, "entrypoint", "web")
	err = os.MkdirAll(webEntrypointDir, 0755)
	if err != nil {
		return err
	}

	return errors.New("TODO start web")
}
