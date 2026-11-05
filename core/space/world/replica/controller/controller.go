// Package space_world_replica_controller runs replica nodes: each keeps one
// SharedObject on its Device in step with the pair's ReplicaSpec, talking to
// the other node of the pair on their peer stream.
package space_world_replica_controller

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world_replica "github.com/s4wave/spacewave/core/space/world/replica"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "sobject/replica"

// retryDelay is how long a node waits to observe again after a failed step
// when no change wakes it first.
const retryDelay = time.Second

// Controller runs one replica node. It serves its ReplicaState to the other
// node, watches the other node's, and takes the step NextStep picks.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// b is the Session's bus.
	b bus.Bus
	// conf is the config.
	conf *space_world_replica.Config
	// local is the peer ID of this node's Device.
	local peer.ID
	// remote is the peer ID of the other node's Device.
	remote peer.ID

	// bcast guards state and peerState, and wakes the reconcile loop and the
	// watchers of state. The watches of the Space and the copy broadcast it
	// too.
	bcast broadcast.Broadcast
	// state is what this node tells the other.
	state *space_world_replica.ReplicaState
	// peerState is what the other node last told, nil while not connected.
	peerState *space_world_replica.ReplicaState
}

// NewController constructs a replica controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *space_world_replica.Config) (*Controller, error) {
	// Parse both peers.
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	local, err := peer.IDB58Decode(conf.GetLocalPeerId())
	if err != nil {
		return nil, err
	}
	remote, err := peer.IDB58Decode(conf.GetRemotePeerId())
	if err != nil {
		return nil, err
	}
	return &Controller{
		le:     le,
		b:      b,
		conf:   conf,
		local:  local,
		remote: remote,
		state:  &space_world_replica.ReplicaState{Spec: conf.GetSpec()},
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "sobject replica controller")
}

// Execute serves and watches the peer stream and reconciles until the context
// ends.
func (c *Controller) Execute(ctx context.Context) error {
	// Find the account hosting this Device's Session.
	acc, release, err := c.resolveAccount(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Serve this node's state and watch the other node's.
	releaseServer, err := c.serve(ctx)
	if err != nil {
		return err
	}
	defer releaseServer()
	go c.watchPeer(ctx)

	// Reconcile until the context ends.
	r := newReconciler(c, acc, acc.GetSessionTransport().GetPrivKey())
	defer r.releaseMount()
	return r.run(ctx)
}

// resolveAccount returns the local provider account whose Session runs as this
// node's Device, waiting for its transport to start.
func (c *Controller) resolveAccount(ctx context.Context) (*provider_local.ProviderAccount, func(), error) {
	// Look up the provider, which must be local.
	prov, provRef, err := provider.ExLookupProvider(ctx, c.b, c.conf.GetProviderId(), false, nil)
	if err != nil {
		return nil, nil, err
	}
	local, ok := prov.(*provider_local.Provider)
	if !ok {
		provRef.Release()
		return nil, nil, errors.Errorf("provider %q is not a local provider, so it cannot move ownership", c.conf.GetProviderId())
	}

	// Wait for the account whose transport runs as the Device.
	acc, release, err := local.WaitSessionAccount(ctx, c.local)
	if err != nil {
		provRef.Release()
		return nil, nil, err
	}
	return acc, func() { release(); provRef.Release() }, nil
}

// wake wakes the reconcile loop after a change it observes.
func (c *Controller) wake() {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
}

// getPeerState returns what the other node last told, or nil.
func (c *Controller) getPeerState() *space_world_replica.ReplicaState {
	var st *space_world_replica.ReplicaState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		st = c.peerState
	})
	return st
}

// getState returns what this node tells the other.
func (c *Controller) getState() *space_world_replica.ReplicaState {
	var st *space_world_replica.ReplicaState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		st = c.state
	})
	return st
}

// setState replaces this node's proof and invite, waking the watchers when
// either changed.
func (c *Controller) setState(proof *space_world_replica.SignedSOReplicaProof, invite *sobject.SOInviteMessage) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.state.GetProof().EqualVT(proof) && c.state.GetInvite().EqualVT(invite) {
			return
		}
		c.state = &space_world_replica.ReplicaState{Spec: c.conf.GetSpec(), Proof: proof, Invite: invite}
		broadcast()
	})
}

// HandleDirective resolves no directives; the stream server serves the peer
// stream.
func (c *Controller) HandleDirective(_ context.Context, _ directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases nothing; Execute releases what it holds when its context
// ends.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
