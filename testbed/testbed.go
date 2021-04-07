package testbed

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	srr "github.com/aperturerobotics/controllerbus/controller/resolver/static"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/core"
	core_test "github.com/aperturerobotics/hydra/core/test"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	"github.com/aperturerobotics/hydra/volume"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
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

// Option is a option passed to NewTestbed
type Option interface{}

type withVolumeConfig struct{ conf config.Config }

// WithVolumeConfig passes a custom volume config to load.
func WithVolumeConfig(conf config.Config) Option {
	return &withVolumeConfig{conf: conf}
}

type withVerbose struct{ verbose bool }

// WithVerbose sets if the verbose mode should be used.
func WithVerbose(verbose bool) Option {
	return &withVerbose{verbose: verbose}
}

// NewTestbed constructs a new core bus with a attached kvtx in-memory volume,
// logger, and other core controllers required for a test to function.
func NewTestbed(ctx context.Context, le *logrus.Entry, opts ...Option) (*Testbed, error) {
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

	/*
		sr.AddFactory(volume_kvtxinmem.NewFactory(b))
		sr.AddFactory(lookup_concurrent.NewFactory(b))
		sr.AddFactory(node_controller.NewFactory(b))
	*/

	core.AddFactories(b, sr)

	verbose := Verbose
	var volumeConfig config.Config
	for _, opt := range opts {
		switch b := opt.(type) {
		case *withVolumeConfig:
			volumeConfig = b.conf
		case *withVerbose:
			verbose = b.verbose
		}
	}
	if volumeConfig == nil {
		volumeConfig = &volume_kvtxinmem.Config{Verbose: verbose}
	}

	dv, _, diRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(
			volumeConfig,
		),
		nil,
	)
	if err != nil {
		return nil, err
	}
	rels = append(rels, diRef.Release)

	_, _, nref, err := loader.WaitExecControllerRunning(
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

	vc := dv.(volume.Controller)
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

// RunSubtest executes t.Run with a sub-test.
func RunSubtest(t *testing.T, name string, cb func(tb *Testbed)) bool {
	return t.Run(name, func(t *testing.T) {
		ctx, ctxCancel := context.WithCancel(context.Background())
		defer ctxCancel()
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		le := logrus.NewEntry(log)
		tb, err := NewTestbed(ctx, le.WithField("subtest", name))
		if err != nil {
			t.Fatal(err.Error())
		}
		cb(tb)
	})
}

// BuildEmptyCursor builds an empty cursor rooted at the volume in the testbed.
func (t *Testbed) BuildEmptyCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	vol := t.Volume
	volID := vol.GetID()
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		t.Bus,
		t.Logger,
		t.StepFactorySet,
		BucketId,
		volID,
		nil, nil,
	)
	return oc, err
}
