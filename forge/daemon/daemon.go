//go:build !js && !wasip1

package daemon

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/forge/core"
	api_controller "github.com/s4wave/spacewave/forge/daemon/api/controller"
	forge_lib "github.com/s4wave/spacewave/forge/lib/all"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/sirupsen/logrus"
)

// Daemon implements the Forge daemon.
type Daemon struct {
	// Peer contains the peer private key.
	peer.Peer
	// ctx is the context.
	ctx context.Context
	// bus is the controller bus.
	bus bus.Bus
	// staticResolver is the static controller factory resolver.
	staticResolver *static.Resolver

	// closeCbs are funcs to call when Close closes the daemon.
	closeCbs []func()
}

// NewDaemon constructs a new daemon.
func NewDaemon(
	ctx context.Context,
	nodePriv crypto.PrivKey,
	opts ConstructOpts,
) (*Daemon, error) {
	// Resolve the Forge daemon logger before constructing controllers.
	le := opts.LogEntry
	if le == nil {
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		le = logrus.NewEntry(log)
	}

	// Establish the daemon lifetime and identify its node peer.
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

	// Make the Forge API controller available to the bus resolver.
	sr.AddFactory(api_controller.NewFactory(b))

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, valRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	le.Info("node controller resolved")

	// Construct the peer controller
	peerCtrl := peer_controller.NewController(le, nodePeer)
	peerCtrlRel, err := b.AddController(ctx, peerCtrl, nil)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	le.Info("node peer controller resolved")

	return &Daemon{
		Peer: nodePeer,

		ctx:            ctx,
		bus:            b,
		staticResolver: sr,
		closeCbs:       []func(){peerCtrlRel, valRef.Release, subCtxCancel},
	}, nil
}

// PrepareConfigSets registers the daemon's controller catalog and loads the ConfigSet controller.
// The caller releases the reference when it no longer needs to apply ConfigSets.
func (d *Daemon) PrepareConfigSets(ctx context.Context) (directive.Reference, error) {
	// Register the Forge runtime and library factories used by ConfigSets.
	core.AddFactories(d.bus, d.staticResolver)
	forge_lib.AddFactories(d.bus, d.staticResolver)

	// Keep the ConfigSet controller running for the caller's scope.
	_, ref, err := d.bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&configset_controller.Config{}), nil)
	return ref, errors.Wrap(err, "construct configset controller")
}

// ServeAPI starts serving the daemon API at addr while the caller holds the reference.
func (d *Daemon) ServeAPI(ctx context.Context, addr string) (directive.Reference, error) {
	_, ref, err := d.bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&api_controller.Config{
		ListenAddr: addr,
	}), nil)
	return ref, errors.Wrap(err, "listen on api")
}

// MergeConfigSetYAML resolves controller types and merges their settings into confSet.
func (d *Daemon) MergeConfigSetYAML(ctx context.Context, data []byte, confSet configset.ConfigSet) error {
	_, err := configset_json.UnmarshalYAML(ctx, d.bus, data, confSet, true)
	return errors.Wrap(err, "unmarshal config yaml")
}

// ApplyConfigSet applies confSet while the caller holds the reference.
func (d *Daemon) ApplyConfigSet(ctx context.Context, confSet configset.ConfigSet) (directive.Reference, error) {
	_, ref, err := d.bus.AddDirective(ctx, configset.NewApplyConfigSet(confSet), nil)
	return ref, err
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
var _ peer.Peer = (*Daemon)(nil)
