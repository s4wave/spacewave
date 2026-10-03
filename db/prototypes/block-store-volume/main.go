package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/core"
	common "github.com/s4wave/spacewave/db/examples/common"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	vc "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/sirupsen/logrus"
)

func Run(ctx context.Context, le *logrus.Entry) error {
	// Start the storage bus used by the prototype volume.
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}

	// Mount the base storage volume for the encrypted wrapper.
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

	// Use the mounted volume controller as the encrypted volume backing store.
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
				false,
				false,
			)
		},
	), nil
	if err != nil {
		panic(err)
	}

	// Execute the encrypted volume controller on the storage bus.
	go func() {
		err := b.ExecuteController(ctx, volCtr)
		if err != nil {
			// fatal error in controller
			panic(err)
		}
	}()

	// Exercise the encrypted volume with the Cayley storage demo.
	le.Info("storage volume(s) resolved")
	if err := common.RunDemoCayley(ctx, le, b, volCtr); err != nil {
		return err
	}

	return nil
}

func main() {
	// Configure debug logging for the encrypted volume prototype.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Run the storage demo and report its failure to the invoking process.
	if err := Run(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}
