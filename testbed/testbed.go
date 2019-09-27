package testbed

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	srr "github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/block/transform/all"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	"github.com/aperturerobotics/hydra/core/test"
	"github.com/aperturerobotics/hydra/node/controller"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/sirupsen/logrus"
)

// BucketId is the id of the test bucket.
var BucketId = "test-bucket-1"

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
	// StepFactorySet is the transformer step factory set.
	StepFactorySet *block_transform.StepFactorySet
	// Release releases the testbed.
	Release func()
}

// Verbose controls if we build verbose testbeds.
var Verbose bool = false

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
			&volume_kvtxinmem.Config{Verbose: Verbose},
		),
		nil,
	)
	if err != nil {
		return nil, err
	}
	rels = append(rels, diRef.Release)

	_, nref, err := bus.ExecOneOff(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(
			&node_controller.Config{},
		),
		nil,
	)
	if err != nil {
		return nil, err
	}
	rels = append(rels, nref.Release)

	vc := dv.GetValue().(volume.Controller)
	v, err := vc.GetVolume(ctx)
	if err != nil {
		t.Release()
		return nil, err
	}
	t.Volume = v
	t.VolumeController = vc

	_, _, _, err = v.PutBucketConfig(&bucket.Config{
		Id:      BucketId,
		Version: 1,
	})
	if err != nil {
		return nil, err
	}

	sfs, err := transform_all.BuildFactorySet()
	if err != nil {
		return nil, err
	}
	t.StepFactorySet = sfs

	return t, nil
}
