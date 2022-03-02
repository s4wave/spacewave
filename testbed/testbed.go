package testbed

import (
	"context"
	"errors"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/aperturerobotics/forge/core"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
)

// Testbed is a constructed testbed.
type Testbed struct {
	*world_testbed.Testbed
}

// NewTestbed constructs a new forge testbed from a Hydra testbed.
func NewTestbed(tb *world_testbed.Testbed) (t *Testbed, tbErr error) {
	if tb == nil {
		return nil, errors.New("testbed cannot be nil")
	}

	t = &Testbed{Testbed: tb}
	b, sr := tb.Bus, tb.StaticResolver

	core.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	return t, nil
}

// Default constructs the default testbed arrangement.
func Default(ctx context.Context, opts ...world_testbed.Option) (*Testbed, error) {
	ttb, err := world_testbed.Default(ctx, opts...)
	if err != nil {
		return nil, err
	}
	tb2, err := NewTestbed(ttb)
	if err != nil {
		ttb.Release()
		return nil, err
	}
	return tb2, nil
}
