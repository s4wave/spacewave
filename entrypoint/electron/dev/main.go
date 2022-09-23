package main

import (
	"context"
	"path"

	"github.com/aperturerobotics/bldr/cli"
	"github.com/aperturerobotics/bldr/entrypoint"
	plugin_fetch "github.com/aperturerobotics/bldr/plugin/fetch"
	"github.com/aperturerobotics/bldr/target/electron"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	// TODO: use the Bldr CLI instead of hardcoding this in init().
	_ "github.com/aperturerobotics/bldr/sandbox"
)

// LogLevel is the default log level to use.
var LogLevel = logrus.DebugLevel

func main() {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	log := logrus.New()
	log.SetLevel(LogLevel)
	le := logrus.NewEntry(log)

	devtoolArgs := &cli.DevtoolArgs{
		Logger:     le,
		UseGitRoot: true,
	}
	devtoolArgs.FillDefaults()

	// get project root
	repoRoot, stateDir, err := devtoolArgs.InitRepoRoot()
	if err != nil {
		le.Fatal(err.Error())
	}
	_ = repoRoot
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the devtool storage & bus
	dtBus, err := cli.BuildDevtoolBus(ctx, le, stateDir)
	if err != nil {
		le.Fatal(err.Error())
	}
	defer dtBus.Release()

	// initialize the root entrypoint plugin
	errCh := make(chan error, 5)
	go func() {
		err := dtBus.ExecStaticPlugin(ctx, le, entrypoint.NewRootPluginInfo(), entrypoint.RootPlugin)
		if err != nil {
			errCh <- err
		}
	}()

	// initialize the fetcher via the root plugin
	_, fetchRef, err := dtBus.GetBus().AddDirective(resolver.NewLoadControllerWithConfig(&plugin_fetch.Config{
		PluginId: entrypoint.RootPlugin.Manifest.GetPluginId(),
	}), nil)
	if err != nil {
		le.Fatal(err.Error())
	}
	defer fetchRef.Release()

	binPath := path.Join(repoRoot, "node_modules/.bin")
	electronPath := path.Join(binPath, "electron")
	workdirPath := repoRoot
	rendererPath := "./build/electron"

	// run the electron runtime controller
	b, sr := dtBus.GetBus(), dtBus.GetStaticResolver()
	sr.AddFactory(electron.NewFactory(b))
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
		le.Fatal(err.Error())
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
		ctxCancel()
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		le.WithError(err).Error("exiting due to error")
	}
	rtRef.Release()
}
