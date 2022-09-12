package main

import (
	"context"
	"path"
	"path/filepath"

	"github.com/aperturerobotics/bldr/core"
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

	// get project root
	repoRoot, err := filepath.Abs("../../../")
	if err != nil {
		le.Fatal(err.Error())
	}
	binPath := path.Join(repoRoot, "node_modules/.bin")
	electronPath := path.Join(binPath, "electron")
	workdirPath := repoRoot
	rendererPath := "./build/electron"

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		le.Fatal(err.Error())
	}
	sr.AddFactory(electron.NewFactory(b))

	// run the browser runtime controller
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
	go func() {
		_ = electron.GetCmd().Wait()
		ctxCancel()
	}()
	<-ctx.Done()
	rtRef.Release()
}
