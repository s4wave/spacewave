package devtool

import (
	"context"
	"os"
	"path"

	plugin_platform "github.com/aperturerobotics/bldr/plugin/platform"
	"github.com/aperturerobotics/bldr/target/electron"
	entrypoint_electron_bundle "github.com/aperturerobotics/bldr/web/entrypoint/electron/bundle"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
)

// ExecuteElectronProject starts the project as an electron app.
func (a *DevtoolArgs) ExecuteElectronProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()

	// execute the project controller
	_, projCtrlRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		true,
		repoRoot,
		a.ConfigPath,
		plugin_platform.PlatformID_NATIVE,
		a.BuildType,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	return b.ExecuteElectron(ctx, repoRoot, a.MinifyEntrypoint, a.BldrVersion, a.BldrVersionSum)
}

// ExecuteElectron starts the application as an electron app.
//
// bldrSum can be empty
func (b *DevtoolBus) ExecuteElectron(ctx context.Context, repoRoot string, minifyEntrypoint bool, bldrVersion, bldrSum string) error {
	if err := b.SyncWebSources(bldrVersion, bldrSum); err != nil {
		return err
	}

	le := b.GetLogger()
	stateDir := b.GetStateRoot()
	webSrcDir := b.GetWebSrcDir()
	entrypointDataDir := path.Join(stateDir, "entry")
	entrypointDir := path.Join(entrypointDataDir, "electron")

	// run esbuild to compile the electron entrypoint
	le.Info("building electron entrypoint")
	entrypoint_electron_bundle.EsbuildLogLevel = esbuild.LogLevelError
	err := entrypoint_electron_bundle.BuildBrowserBundle(le, webSrcDir, entrypointDir, minifyEntrypoint)
	if err != nil {
		return err
	}

	// link node_modules to the project root to fix electron devtools
	nodeModulesDest := path.Join(entrypointDir, "node_modules")
	nodeModulesSrc := path.Join(repoRoot, "node_modules")
	if _, err := os.Stat(nodeModulesDest); os.IsNotExist(err) {
		if err := os.Symlink(nodeModulesSrc, nodeModulesDest); err != nil {
			le.WithError(err).Warn("failed to symlink node_modules to project root")
		}
	}

	// launch electron
	binPath := path.Join(repoRoot, "node_modules/.bin")
	electronPath := path.Join(binPath, "electron")
	if _, err := os.Stat(electronPath); err != nil {
		return errors.Wrap(err, "please install Electron: npm install --dev electron")
	}

	workdirPath := repoRoot
	rendererPath := entrypointDir

	// run the electron runtime controller
	bb, sr := b.GetBus(), b.GetStaticResolver()
	sr.AddFactory(electron.NewFactory(bb))

	// start electron controller
	ctrl, _, rtRef, err := loader.WaitExecControllerRunning(
		ctx,
		bb,
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

	// write the banner
	writeBanner()

	// shutdown program if electron exits.
	le.Info("electron is running")
	go func() {
		_ = electron.GetCmd().Wait()
		b.Release()
	}()

	<-b.GetContext().Done()
	rtRef.Release()
	return nil
}
