package daemon

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/core"
	core_all "github.com/s4wave/spacewave/db/core/all"
	api_controller "github.com/s4wave/spacewave/db/daemon/api/controller"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/sirupsen/logrus"
)

// Daemon implements the Hydra daemon.
type Daemon struct {
	// Peer contains the peer private key.
	peer.Peer
	// ctx is the context.
	ctx context.Context
	// bus is the controller bus.
	bus bus.Bus

	// closeCbs are funcs to call when the daemon closes.
	closeCbs []func()
}

// NewDaemon constructs a new daemon.
func NewDaemon(
	ctx context.Context,
	nodePriv crypto.PrivKey,
	opts ConstructOpts,
) (*Daemon, error) {
	// Resolve the daemon logger.
	le := opts.LogEntry
	if le == nil {
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		le = logrus.NewEntry(log)
	}

	// Create the daemon context and local peer identity.
	ctx, subCtxCancel := context.WithCancel(ctx)
	nodePeer, err := peer.NewPeer(nodePriv)
	if err != nil {
		subCtxCancel()
		return nil, err
	}

	// Construct the controller bus and register all factories.
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	core_all.AddFactories(b, sr)

	// Resolve the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, valRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	le.Info("node controller resolved")

	// Add and retain the peer controller.
	peerCtrl := peer_controller.NewController(le, nodePeer)
	peerCtrlRel, err := b.AddController(ctx, peerCtrl, nil)
	if err != nil {
		subCtxCancel()
		return nil, err
	}
	le.Info("node peer controller resolved")

	// Return the daemon with all release callbacks.
	return &Daemon{
		Peer: nodePeer,

		ctx:      ctx,
		bus:      b,
		closeCbs: []func(){peerCtrlRel, valRef.Release, subCtxCancel},
	}, nil
}

// PrepareConfigSets loads the ConfigSet controller using the factories registered at construction.
// The caller releases the reference when it no longer needs to apply ConfigSets.
func (d *Daemon) PrepareConfigSets(ctx context.Context) (directive.Reference, error) {
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
	// Detach the release callbacks before invoking them.
	closeCbs := d.closeCbs
	d.closeCbs = nil

	// Invoke each detached release callback.
	for _, cb := range closeCbs {
		cb()
	}
}

// _ is a type assertion
var _ peer.Peer = (*Daemon)(nil)
