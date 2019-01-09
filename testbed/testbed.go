package testbed

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	srr "github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	"github.com/aperturerobotics/hydra/core/test"
	"github.com/aperturerobotics/hydra/node/controller"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/sirupsen/logrus"
)

// Testbed is a constructed testbed.
type Testbed struct {
	// Context is the root context.
	Context context.Context
	// Logger is the logger
	Logger *logrus.Entry
	// VolumeController is the test volume controller.
	VolumeController volume.Controller
	// Volume is the test volume.
	Volume volume.Volume
	// StaticResolver is the static resolver.
	StaticResolver *srr.Resolver
	// Bus is the controller bus
	Bus bus.Bus
	// Release releases the testbed.
	Release func()
}

// NewTestbed constructs a new core bus with a attached kvtx in-memory volume,
// logger, and other core controllers required for a test to function.
func NewTestbed(ctx context.Context, le *logrus.Entry) (*Testbed, error) {
	var rels []func()
	t := &Testbed{
		Context: ctx,
		Logger:  le,
		Release: func() {
			for _, rel := range rels {
				rel()
			}
		},
	}

	b, sr, err := core_test.NewTestingBus(ctx, le)
	if err != nil {
		return nil, err
	}
	t.StaticResolver = sr
	t.Bus = b
	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(node_controller.NewFactory(b))

	// create a kvtx inmem setup
	dv, diRef, err := bus.ExecOneOff(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(
			&volume_kvtxinmem.Config{Verbose: true},
		),
		nil,
	)
	if err != nil {
		return nil, err
	}
	rels = append(rels, diRef.Release)

	vc := dv.GetValue().(volume.Controller)
	v, err := vc.GetVolume(ctx)
	if err != nil {
		t.Release()
		return nil, err
	}
	t.Volume = v
	t.VolumeController = vc

	return t, nil
}
