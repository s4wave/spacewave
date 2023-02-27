package devtool

import (
	"context"

	"github.com/pkg/errors"
)

// DistProject builds a dist bundle of the project to dist/ with the given platform ID.
func (a *DevtoolArgs) DistProject(ctx context.Context) error {
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

	if err := b.SyncWebSources(a.BldrVersion, a.BldrVersionSum); err != nil {
		return err
	}
	defer b.Release()

	// write the banner
	writeBanner()

	// execute the project controller
	// compiles the plugins and stores them in the devtool bus world
	_, projCtrlRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		false,
		repoRoot,
		a.ConfigPath,
		a.PlatformID,
		a.BuildType,
	)
	if err != nil {
		return err
	}
	defer projCtrlRef.Release()

	// TODO: wait for plugins to finish compiling (or fail)
	// TODO: embed the initial plugin versions as embedded data in the entrypoint
	// TODO: build the distribution entrypoint for the given PlatformID

	// run esbuild to compile the electron entrypoint
	/*
		webSrcDir := b.GetWebSrcDir()
		entrypointDataDir := path.Join(stateDir, "entry")
		entrypointDir := path.Join(entrypointDataDir, "electron")
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


		// shutdown program if electron exits.
		le.Info("electron is running")
		go func() {
			_ = electron.GetCmd().Wait()
			b.Release()
		}()

		<-b.GetContext().Done()
		rtRef.Release()
	*/
	return errors.New("TODO bundle distribution entrypoint")
}
