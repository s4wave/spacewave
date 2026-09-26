package testbed

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	srr "github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/core"
	core_test "github.com/s4wave/spacewave/db/core/test"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
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
	// BucketId is the id of the test bucket.
	BucketId string
	// StaticResolver is the static resolver.
	StaticResolver *srr.Resolver
	// Bus is the controller bus
	Bus bus.Bus
	// StepFactorySet is the transformer step factory set.
	StepFactorySet *block_transform.StepFactorySet

	// rels contains the set of functions to call on Release.
	rels []func()
}

// Verbose controls if we build verbose testbeds.
var Verbose bool

// Option is an option passed to NewTestbed.
type Option any

// withVolumeConfig selects the volume config to load.
type withVolumeConfig struct{ conf config.Config }

// WithVolumeConfig passes a custom volume config to load.
func WithVolumeConfig(conf config.Config) Option {
	return &withVolumeConfig{conf: conf}
}

// withVerbose selects verbose volume logging.
type withVerbose struct{ verbose bool }

// WithVerbose sets if the verbose mode should be used.
func WithVerbose(verbose bool) Option {
	return &withVerbose{verbose: verbose}
}

// NewTestbed constructs a new core bus with an attached kvtx in-memory volume,
// logger, and other core controllers required for a test to function.
func NewTestbed(ctx context.Context, le *logrus.Entry, opts ...Option) (tb *Testbed, tbErr error) {
	// Release partially initialized controllers when setup fails.
	var rels []func()
	defer func() {
		if tbErr != nil {
			release(rels)
		}
	}()

	// Construct the testing bus and register core factories.
	b, sr, err := core_test.NewTestingBus(ctx, le)
	if err != nil {
		return nil, err
	}

	core.AddFactories(b, sr)

	// Start the config-set controller.
	_, _, csRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "construct configset controller")
	}
	rels = append(rels, csRef.Release)

	// Resolve testbed volume options.
	var volumeConfig config.Config
	var volumeConfigEmpty bool
	verbose := Verbose
	for _, opt := range opts {
		switch o := opt.(type) {
		case *withVolumeConfig:
			volumeConfig = o.conf
			if o.conf == nil {
				volumeConfigEmpty = true
			}
		case *withVerbose:
			verbose = o.verbose
		}
	}
	if volumeConfig == nil && !volumeConfigEmpty {
		volumeConfig = &volume_kvtxinmem.Config{
			Verbose:      verbose,
			VolumeConfig: &volume_controller.Config{},
		}
	}

	// Start the node controller.
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

	// Start the configured volume and apply the test bucket.
	var vc volume.Controller
	var v volume.Volume
	var bucketID string
	if !volumeConfigEmpty {
		var relVolume func()
		vc, relVolume, err = startVolume(ctx, b, le, volumeConfig)
		if err != nil {
			return nil, err
		}
		rels = append(rels, relVolume)

		v, err = vc.GetVolume(ctx)
		if err != nil {
			return nil, err
		}
		if bucketID == "" {
			bucketID = "test-bucket"
		}
		_, _, _, err = v.ApplyBucketConfig(ctx, &bucket.Config{
			Id:  bucketID,
			Rev: 1,
		})
		if err != nil {
			return nil, err
		}
	}

	// Build the transform factory set and return the testbed.
	sfs := transform_all.BuildFactorySet()

	return &Testbed{
		Context: ctx,
		Logger:  le,
		rels:    rels,

		Bus:              b,
		Volume:           v,
		BucketId:         bucketID,
		VolumeController: vc,
		StepFactorySet:   sfs,
		StaticResolver:   sr,
	}, nil
}

// RunTest executes a test.
func RunTest(t *testing.T, cb func(t *testing.T, tb *Testbed)) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	tb, err := NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	cb(t, tb)
}

// RunSubtest executes t.Run with a sub-test.
//
// Run runs f as a subtest of t called name. It runs f in a separate goroutine
// and blocks until f returns or calls t.Parallel to become a parallel test.
// Run reports whether f succeeded (or at least did not fail before calling t.Parallel).
//
// Run may be called simultaneously from multiple goroutines, but all such calls
// must return before the outer test function for t returns.
func RunSubtest(t *testing.T, name string, cb func(t *testing.T, tb *Testbed)) bool {
	return t.Run(name, func(t *testing.T) {
		ctx := t.Context()
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		le := logrus.NewEntry(log)
		tb, err := NewTestbed(ctx, le.WithField("subtest", name))
		if err != nil {
			t.Fatal(err.Error())
		}
		defer tb.Release()
		cb(t, tb)
	})
}

// BuildEmptyCursor builds an empty cursor rooted at the volume in the testbed.
func (t *Testbed) BuildEmptyCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	vol := t.Volume
	if vol == nil {
		return nil, errors.New("no testbed volume configured")
	}

	volID := vol.GetID()
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		t.Bus,
		t.Logger,
		t.StepFactorySet,
		t.BucketId,
		volID,
		nil, nil,
	)
	return oc, err
}

// AddReleaseFunc adds a function to call when Release() is called.
func (t *Testbed) AddReleaseFunc(cb func()) {
	t.rels = append(t.rels, cb)
}

// Release calls all release functions, newest first. It returns after the
// testbed volume controller has exited and closed its store, so a test may
// remove the volume's files afterward.
func (t *Testbed) Release() {
	rs := t.rels
	t.rels = nil
	release(rs)
}

// release calls rels newest first, so each runs before the releases of the
// controllers it was built on.
func release(rels []func()) {
	for _, rel := range slices.Backward(rels) {
		rel()
	}
}

// startVolume constructs the volume controller for conf and attaches it to the
// bus. The returned release waits for the controller to exit and close.
func startVolume(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	conf config.Config,
) (volume.Controller, func(), error) {
	factory, factoryRef, err := resolver.ExLoadFactoryByConfig(ctx, b, conf)
	if err != nil {
		return nil, nil, errors.Wrap(err, "load volume factory")
	}
	defer factoryRef.Release()

	ctrl, err := factory.Construct(ctx, conf, controller.ConstructOpts{
		Logger: le.WithField("config", conf.GetConfigID()),
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "construct volume controller")
	}
	vc, ok := ctrl.(volume.Controller)
	if !ok {
		// The controller never ran, so Close has nothing to report.
		_ = ctrl.Close()
		return nil, nil, errors.Errorf("config %s is not a volume controller", conf.GetConfigID())
	}
	rel, err := b.AddController(ctx, vc, nil)
	if err != nil {
		// The bus rejected the controller before running it.
		_ = vc.Close()
		return nil, nil, err
	}
	return vc, rel, nil
}
