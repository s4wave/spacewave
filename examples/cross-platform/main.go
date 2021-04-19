package main

import (
	"context"
	"os"
	"runtime"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/core"
	common "github.com/aperturerobotics/hydra/examples/common"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

func Run(ctx context.Context, le *logrus.Entry) error {
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}

	sr.AddFactory(reconciler_example.NewFactory(b))

	// TODO: add storage depending on if we are in js or not.
	verbose := false
	av, _, ref, err := common.AddStorageVolume(ctx, le, b, sr, verbose)
	if err != nil {
		return err
	}
	defer ref.Release()

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, ncRef, err := loader.WaitExecControllerRunning(ctx, b, dir, nil)
	if err != nil {
		return err
	}
	defer ncRef.Release()
	le.Info("node controller resolved")

	le.Info("storage volume resolved")
	volCtr := av.(volume.Controller)

	if err := common.RunDemoCayley(ctx, le, b, volCtr); err != nil {
		return err
	}

	cloneURL := "../../"
	if runtime.GOOS == "js" {
		// clone from the proxy (see ./proxy)
		// we could clone from GitHub, but they don't set cross-origin headers.
		cloneURL = "http://localhost:5000/.git/"
	}
	if err := common.RunDemoGit(ctx, le, b, volCtr, cloneURL); err != nil {
		le.WithError(err).Error("git demo failed")
	}

	return nil
}

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	if err := Run(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}
