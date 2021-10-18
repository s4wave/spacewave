package assembly_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/sirupsen/logrus"
)

// NewSubAssemblyBus constructs a new sub-assembly bus.
func NewSubAssemblyBus(ctx context.Context, le *logrus.Entry) (bus.Bus, *static.Resolver, error) {
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, nil, err
	}
	sr.AddFactory(NewFactory(b))
	return b, sr, err
}
