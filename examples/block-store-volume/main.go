package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/core"
	common "github.com/aperturerobotics/hydra/examples/common"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume"
	vc "github.com/aperturerobotics/hydra/volume/controller"
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
	av, _, svolRef, err := common.AddStorageVolume(ctx, le, b, sr, verbose)
	if err != nil {
		return err
	}
	defer svolRef.Release()

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, ncRef, err := loader.WaitExecControllerRunning(ctx, b, dir, nil)
	if err != nil {
		return err
	}
	defer ncRef.Release()
	le.Info("node controller resolved")

	le.Info("storage volume resolved")
	baseVolCtr := av.(volume.Controller)

	// Construct wrapper for base storage volume.
	vcConfig := &vc.Config{}
	volCtr, err := vc.NewController(
		le,
		vcConfig,
		b,
		controller.NewInfo(
			ControllerID,
			Version,
			"encrypted volume test",
		),
		func(
			ctx context.Context,
			le *logrus.Entry,
		) (volume.Volume, error) {
			return NewEncryptedVolume(
				ctx,
				b,
				le,
				baseVolCtr,
				nil, // nil kvtx store config
				nil, // nil kvkey config
			)
		},
	), nil
	if err != nil {
		panic(err)
	}
	go func() {
		err := b.ExecuteController(ctx, volCtr)
		if err != nil {
			// fatal error in controller
			panic(err)
		}
	}()

	le.Info("storage volume(s) resolved")
	if err := common.RunDemoCayley(ctx, le, b, volCtr); err != nil {
		return err
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
