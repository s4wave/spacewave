package main

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/core"
	common "github.com/aperturerobotics/hydra/examples/common"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume"
	vc "github.com/aperturerobotics/hydra/volume/controller"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/sirupsen/logrus"
)

// Overlay a block graph (iavl tree) on top of a underlying store.

// Create a volume on top of that iavl tree to produce a "block-graph backed" volume.

// Eventually this could be used to implement a cloud volume controlled by a anchor chain.

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		panic(err)
	}

	// disable pin controller
	vcConfig := &vc.Config{
		// DisablePin: true,
	}

	sr.AddFactory(reconciler_example.NewFactory(b))
	sr.AddFactory(volume_kvtxinmem.NewFactory(b))

	var ref directive.Reference
	var baseStorageVolAv controller.Controller

	useInMemory := false
	verbose := false
	if useInMemory {
		baseStorageVolAv, _, ref, err = loader.WaitExecControllerRunning(
			ctx,
			b,
			resolver.NewLoadControllerWithConfig(&volume_kvtxinmem.Config{
				Verbose:      verbose, // show what's going on under the hood.
				VolumeConfig: vcConfig,
			}),
			nil,
		)
	} else {
		baseStorageVolAv, _, ref, err = common.AddStorageVolume(ctx, le, b, sr, verbose)
	}
	if err != nil {
		panic(err)
	}
	defer ref.Release()
	baseStorageVolCtrl := baseStorageVolAv.(volume.Controller)

	// Construct wrapper for base storage volume.
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
				baseStorageVolCtrl,
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

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, ncRef, err := loader.WaitExecControllerRunning(ctx, b, dir, nil)
	if err != nil {
		panic(err)
	}
	defer ncRef.Release()
	le.Info("node controller resolved")

	le.Info("storage volume(s) resolved")
	if err := common.RunDemoCayley(ctx, le, b, volCtr); err != nil {
		panic(err)
	}
}
