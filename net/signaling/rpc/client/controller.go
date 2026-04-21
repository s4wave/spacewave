package signaling_rpc_client

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/signaling/rpc/client"

// Controller is the signaling client controller.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// conf is the config
	conf *Config
	// rpcClient is the stream srpc client
	rpcClient stream_srpc_client.Client
	// srv is the service client
	srv signaling_rpc.SRPCSignalingClient
	// sessionTrackers contains a mapping from peer id to ongoing session
	// peer id is encoded as string
	sessionTrackers *keyed.KeyedRefCount[string, *sessionTracker]
	// listenSessions is a list of sessions that were initiated by remote peers.
	// NOTE: managed by the callback from the client listen handler ONLY.
	listenSessions map[string]*keyed.KeyedRef[string, *sessionTracker]
	// client is the signaling client
	// nil until resolved by Execute
	client *ccontainer.CContainer[*Client]
}

// NewController constructs a new signaling client controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) (*Controller, error) {
	// determine protocol id
	protocolID, err := conf.ParseProtocolID()
	if err != nil {
		return nil, err
	}
	if protocolID == "" {
		protocolID = signaling_rpc.ProtocolID
	}

	// determine service id
	serviceID := conf.GetServiceId()
	if serviceID == "" {
		serviceID = signaling_rpc.SRPCSignalingServiceID
	}

	// construct rpc client
	rpcClient, err := stream_srpc_client.NewClient(le, b, conf.GetClient(), protocolID)
	if err != nil {
		return nil, err
	}

	// construct rpc service
	srv := signaling_rpc.NewSRPCSignalingClientWithServiceID(rpcClient, serviceID)

	// construct controller
	c := &Controller{
		le:             le,
		b:              b,
		conf:           conf,
		rpcClient:      rpcClient,
		srv:            srv,
		listenSessions: make(map[string]*keyed.KeyedRef[string, *sessionTracker]),
		client:         ccontainer.NewCContainer[*Client](nil),
	}

	// The session tracker starts when we want a session with a remote peer.
	c.sessionTrackers = keyed.NewKeyedRefCount[string, *sessionTracker](
		c.newSessionTracker,
		keyed.WithExitLogger[string, *sessionTracker](le),
		keyed.WithBackoff[string, *sessionTracker](func(_ string) cbackoff.BackOff {
			return conf.GetBackoff().Construct()
		}),
	)

	// return controller
	return c, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "signaling rpc client")
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Validate asserts that this won't return an error.
	localPeerID, err := c.conf.ParsePeerID()
	if err != nil {
		return err
	}

	c.le.
		WithField("peer-id", localPeerID.String()).
		Debug("waiting for peer private key")
	localPeer, _, localPeerRef, err := peer.GetPeerWithID(ctx, c.b, localPeerID, false, nil)
	if err != nil {
		return err
	}

	// Get the priv key and release the peer
	privKey, err := localPeer.GetPrivKey(ctx)
	localPeerRef.Release()
	if err != nil {
		return err
	}

	// Construct the signaling client
	signalingClient, err := NewClient(c.le, c.srv, privKey, c.conf.GetBackoff())
	if err != nil {
		return err
	}

	// Set the signaling client
	c.client.SetValue(signalingClient)

	// Set the listening routine if applicable
	if !c.conf.GetDisableListen() {
		signalingClient.SetListenHandler(c.handlePeerWantsSession)
	}

	// Set the contexts, starting the client.
	signalingClient.SetContext(ctx)
	c.sessionTrackers.SetContext(ctx, true)

	// Done
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The passed context is canceled when the directive instance expires.
// NOTE: the passed context is not canceled when the handler is removed.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case signaling.SignalPeer:
		return c.resolveSignalPeer(ctx, di, dir)
	}
	return nil, nil
}

// handlePeerWantsSession handles when the list of peers that want a session changes.
func (c *Controller) handlePeerWantsSession(ctx context.Context, reset, added bool, pid string) {
	delSession := func(delPeerID string) {
		ref, ok := c.listenSessions[delPeerID]
		if ok {
			ref.Release()
			delete(c.listenSessions, delPeerID)
		}
	}
	addSession := func(addPeerID string) {
		_, ok := c.listenSessions[addPeerID]
		if ok {
			// already existed
			return
		}

		ref, _, _, err := c.addSessionTrackerRef(ctx, addPeerID)
		if err != nil {
			if err != context.Canceled {
				c.le.
					WithField("remote-peer", addPeerID).
					WithError(err).
					Warn("invalid incoming session peer id")
			}
			return
		}
		c.le.
			WithField("remote-peer", addPeerID).
			Debug("added incoming session")
		c.listenSessions[addPeerID] = ref
	}

	if reset {
		for delPeerID := range c.listenSessions {
			delSession(delPeerID)
		}
	}

	if pid != "" {
		if added {
			addSession(pid)
		} else {
			delSession(pid)
		}
	}
}

// addSessionTrackerRef validates the peer id string and adds a session tracker ref.
func (c *Controller) addSessionTrackerRef(
	ctx context.Context,
	peerIDStr string,
) (*keyed.KeyedRef[string, *sessionTracker], *sessionTracker, bool, error) {
	// assert that we can extract the public key from the peer id
	peerID, peerPub, err := peer.ParsePeerIDWithPubKey(peerIDStr)
	if err != nil {
		return nil, nil, false, err
	}

	// wait for the client to be ready
	client, err := c.client.WaitValue(ctx, nil)
	if err != nil {
		return nil, nil, false, err
	}

	// assert that we are not trying to open a session with ourselves
	if client.peerID.MatchesPublicKey(peerPub) {
		return nil, nil, false, errors.New("signaling: cannot self-dial")
	}

	ref, tkr, existed := c.sessionTrackers.AddKeyRef(peerID.String())
	return ref, tkr, existed, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.sessionTrackers.ClearContext()
	c.client.SetValue(nil)
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
