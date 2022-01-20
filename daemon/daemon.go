package daemon

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	peer_controller "github.com/aperturerobotics/bifrost/peer/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/hydra/core"
	core_all "github.com/aperturerobotics/hydra/core/all"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/sirupsen/logrus"
)

// Daemon implements the Hydra daemon.
type Daemon struct {
	// Peer contains the peer private key
	peer.Peer
	// ctx is the context
	ctx context.Context
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

	ctx, subCtxCancel := context.WithCancel(ctx)
	nodePeer, err := peer.NewPeer(nodePriv)
	if err != nil {
		subCtxCancel()
		return nil, err
	}

	// Construct the controller bus.
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	core_all.AddFactories(b, sr)

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, valRef, err := bus.ExecOneOff(ctx, b, dir, false, nil)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	le.Info("node controller resolved")

	// Construct the peer controller
	peerCtrl, err := peer_controller.NewController(le, nodePriv)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	go b.ExecuteController(ctx, peerCtrl)
	le.Info("node peer controller resolved")

	return &Daemon{
		Peer: nodePeer,

		ctx:            ctx,
		bus:            b,
		staticResolver: sr,
		closeCbs:       []func(){valRef.Release, subCtxCancel},
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

// Close calls all close callbacks.
func (d *Daemon) Close() {
	closeCbs := d.closeCbs
	d.closeCbs = nil
	for _, cb := range closeCbs {
		cb()
	}
}

// _ is a type assertion
var _ peer.Peer = ((*Daemon)(nil))
