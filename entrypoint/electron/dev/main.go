package main

import (
	"context"
	"path"

	"github.com/aperturerobotics/bldr/cli"
	"github.com/aperturerobotics/bldr/target/electron"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

	<-ctx.Done()
	rtRef.Release()
}
