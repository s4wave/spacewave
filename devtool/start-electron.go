package devtool

import (
	"context"
	"os"
	"path"

	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/entrypoint/electron/bundle"
	"github.com/aperturerobotics/bldr/target/electron"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
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

	if err := dtBus.SyncWebSources(); err != nil {
		return err
	}

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

	// initialize the bldr start

	// TODO: initialize plugin compiler from config file
	// TODO: load root plugins from config file

	// launch electron
	binPath := path.Join(repoRoot, "node_modules/.bin")
	electronPath := path.Join(binPath, "electron")
	if _, err := os.Stat(electronPath); err != nil {
		return errors.Wrap(err, "please install Electron: npm install --dev electron")
	}

	workdirPath := repoRoot
	rendererPath := entrypointDir

	// run the electron runtime controller
	b, sr := dtBus.GetBus(), dtBus.GetStaticResolver()
	sr.AddFactory(electron.NewFactory(b))

	// start electron controller
	ctrl, _, rtRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&electron.Config{
			ElectronPath: electronPath,
			RendererPath: rendererPath,
			WorkdirPath:  workdirPath,
		}),
		nil,
	)
	if err != nil {
		err = errors.Wrap(err, "start runtime controller")
		return err
	}
	electronCtrl := ctrl.(*electron.Controller)
	electron, err := electronCtrl.WaitElectron(ctx, nil)
	if err != nil {
		err = errors.Wrap(err, "get started electron")
		le.Fatal(err.Error())
	}

	// shutdown program if electron exits.
	le.Info("electron is running")
	go func() {
		_ = electron.GetCmd().Wait()
		dtBus.Release()
	}()

	<-dtBus.GetContext().Done()
	rtRef.Release()
	return nil
}
