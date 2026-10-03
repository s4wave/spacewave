package testbed

import (
	"context"
	"errors"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/forge/core"
)

// Testbed is a constructed testbed.
type Testbed struct {
	*world_testbed.Testbed
}

// NewTestbed constructs a new forge testbed from a Hydra testbed.
func NewTestbed(tb *world_testbed.Testbed) (t *Testbed, tbErr error) {
	// Require an existing World testbed before installing forge support.
	if tb == nil {
		return nil, errors.New("testbed cannot be nil")
	}

	// Extend the World testbed using its bus and static resolver.
	t = &Testbed{Testbed: tb}
	b, sr := tb.Bus, tb.StaticResolver

	// Register forge and boilerplate controller factories on the testbed bus.
	core.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	return t, nil
}

// Default constructs the default testbed arrangement.
func Default(ctx context.Context, opts ...world_testbed.Option) (*Testbed, error) {
	// Construct the default World testbed for forge operations.
	ttb, err := world_testbed.Default(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// Install forge support and release the World testbed if setup fails.
	tb2, err := NewTestbed(ttb)
	if err != nil {
		ttb.Release()
		return nil, err
	}
	return tb2, nil
}

// WithTestbedOptions constructs the testbed with the given testbed options.
func WithTestbedOptions(ctx context.Context, testbedOptions []hydra_testbed.Option, worldOpts []world_testbed.Option) (*Testbed, error) {
	// Construct the World testbed with the requested storage and World options.
	tb, err := world_testbed.WithTestbedOptions(ctx, testbedOptions, worldOpts)
	if err != nil {
		return nil, err
	}

	// Install forge support and release the World testbed if setup fails.
	tb2, err := NewTestbed(tb)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}
