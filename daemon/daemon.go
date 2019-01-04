//+build !js

package daemon

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/hydra/core"
	"github.com/aperturerobotics/hydra/daemon/api/controller"
	"github.com/aperturerobotics/hydra/node/controller"
	"github.com/libp2p/go-libp2p-crypto"
	"github.com/sirupsen/logrus"
)

// Daemon implements the Hydra daemon.
type Daemon struct {
	// bus is the controller bus.
	bus bus.Bus
	// staticResolver is the static controller factory resolver.
	staticResolver *static.Resolver

	// closeCbs are funcs to call when we close the daemon
	closeCbs []func()
}

// ConstructOpts are extra options passed to the daemon constructor.
type ConstructOpts struct {
	// LogEntry is the root logger to use.
	// If unset, will use a default logger.
	LogEntry *logrus.Entry
	// ExtraControllerFactories is a set of extra controller factories to
	// make available to the daemon.
	ExtraControllerFactories []func(bus.Bus) controller.Factory
}

// NewDaemon constructs a new daemon.
func NewDaemon(
	ctx context.Context,
	nodePriv crypto.PrivKey,
	opts ConstructOpts,
) (*Daemon, error) {
	le := opts.LogEntry
	if le == nil {
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		le = logrus.NewEntry(log)
	}

	// Construct the controller bus.
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, err
	}

	sr.AddFactory(api_controller.NewFactory(b))

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, valRef, err := bus.ExecOneOff(ctx, b, dir, nil)
	if err != nil {
		return nil, err
	}
	le.Info("node controller resolved")

	return &Daemon{
		bus: b,

		closeCbs:       []func(){valRef.Release},
		staticResolver: sr,
	}, nil
}

// GetStaticResolver returns the underlyino static resolver for controller impl lookups.
func (d *Daemon) GetStaticResolver() *static.Resolver {
	return d.staticResolver
}

// GetControllerBus returns the controller bus.
func (d *Daemon) GetControllerBus() bus.Bus {
	return d.bus
}
