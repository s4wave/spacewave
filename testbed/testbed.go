package testbed

import (
	"context"
	"errors"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/aperturerobotics/forge/core"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/sirupsen/logrus"
)

// Testbed is a constructed testbed.
type Testbed struct {
	*world_testbed.Testbed
}

// NewTestbed constructs a new forge testbed from a Hydra testbed.
func NewTestbed(tb *world_testbed.Testbed, opts ...Option) (t *Testbed, tbErr error) {
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
func Default(ctx context.Context) (*Testbed, error) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := world_testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, err
	}
	tb2, err := NewTestbed(tb)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}
