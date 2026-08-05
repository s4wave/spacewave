package core_test

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	nctr "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/sirupsen/logrus"
)

// NewTestingBus constructs a minimal in-memory Hydra bus stack.
func NewTestingBus(
	ctx context.Context,
	le *logrus.Entry,
	opts ...cbc.Option,
) (bus.Bus, *static.Resolver, error) {
	// Construct the minimal controller bus.
	b, sr, err := cbc.NewCoreBus(ctx, le, opts...)
	if err != nil {
		return nil, nil, err
	}

	// Register the network peer factory used by tests.
	sr.AddFactory(nctr.NewFactory(b))
	return b, sr, nil
}
