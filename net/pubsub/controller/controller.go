package pubsub_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a PubSub with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	peer peer.Peer,
	handler pubsub.PubSubHandler,
) (pubsub.PubSub, error)

// Controller implements a common PubSub controller. The controller monitors
// active links, and pushes peer ID <-> link UUID tuples to the router. It
// handles PubSub event callbacks and PubSub related directives, yielding PubSub
// client handles and managing active topics.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor Constructor
	// info is the controller info
	info *controller.Info
	// protocolID is the protocol id
	protocolID protocol.ID
	// peerID is the peer ID to use
	peerID peer.ID

	// pubSubCtr holds the PubSub
	pubSubCtr *ccontainer.CContainer[*pubsub.PubSub]
	// peerCtr holds the peer
	peerCtr *ccontainer.CContainer[*peer.Peer]

	// bcast guards below fields
	bcast broadcast.Broadcast
	// cleanupRefs are the refs to cleanup
	cleanupRefs []directive.Reference
	// incLinks are links that need to be established
	incLinks []link.MountedLink
	// links are tracked links
	links map[pubsub.PeerLinkTuple]*trackedLink
}

// NewController constructs a new transport controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	controllerInfo *controller.Info,
	peerID peer.ID,
	protocolID protocol.ID,
	ctor Constructor,
) *Controller {
	return &Controller{
		le:     le,
		bus:    bus,
		ctor:   ctor,
		info:   controllerInfo,
		peerID: peerID,

		protocolID: protocolID,
		pubSubCtr:  ccontainer.NewCContainer[*pubsub.PubSub](nil),
		peerCtr:    ccontainer.NewCContainer[*peer.Peer](nil),
		links:      make(map[pubsub.PeerLinkTuple]*trackedLink),
	}
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return c.info.GetId()
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the transport controller and the transport.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Fetch the peer if the peer ID is set.
	var cpeer peer.Peer
	var err error
	if len(c.peerID) != 0 {
		// special value: "any"
		if c.peerID == peer.ID("any") {
			c.peerID = peer.ID("")
		}

		var ref directive.Reference
		cpeer, _, ref, err = peer.GetPeerWithID(ctx, c.bus, c.peerID, false, nil)
		if err != nil {
			return err
		}
		if ref != nil {
			defer ref.Release()
		}
		c.peerID = cpeer.GetPeerID()
	}

	c.peerCtr.SetValue(&cpeer)
	defer c.peerCtr.SetValue(nil)

	// Construct the PubSub
	ps, err := c.ctor(
		ctx,
		c.le,
		cpeer,
		c,
	)
	if err != nil {
		return err
	}
	defer ps.Close()
	c.pubSubCtr.SetValue(&ps)

	psErr := make(chan error, 1)
	go func() {
		c.le.Debug("executing pubsub")
		if err := ps.Execute(ctx); err != nil {
			psErr <- err
		}
	}()

	for {
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			for _, vl := range c.incLinks {
				tpl := pubsub.NewPeerLinkTuple(vl)
				if e, ok := c.links[tpl]; ok {
					e.ctxCancel()
				}
				tlCtx, tlCtxCancel := context.WithCancel(ctx)
				tl := &trackedLink{
					c:         c,
					ctxCancel: tlCtxCancel,
					tpl:       tpl,
					lnk:       vl,
					le: c.le.
						WithField("link-uuid", vl.GetLinkUUID()).
						WithField("link-remote-peer", vl.GetRemotePeer().String()),
				}
				c.links[tpl] = tl
				go func() {
					err := tl.trackLink(tlCtx)
					tlCtxCancel()
					if err != context.Canceled && err != nil {
						tl.le.WithError(err).Warn("link tracker returned fatal error")
					}
					c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
						if ol, ok := c.links[tpl]; ok && ol == tl {
							delete(c.links, tpl)
						}
					})
				}()
			}
			c.incLinks = nil
			waitCh = getWaitCh()
		})

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-psErr:
			return err
		case <-waitCh:
		}
	}
}

// GetPubSub returns the controlled PubSub.
func (c *Controller) GetPubSub(ctx context.Context) (pubsub.PubSub, error) {
	val, err := c.pubSubCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *val, nil
}

// GetPeer returns the controlled peer ID.
func (c *Controller) GetPeer(ctx context.Context) (peer.Peer, error) {
	val, err := c.peerCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *val, nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case link.EstablishLinkWithPeer:
		c.handleEstablishLink(di)
	case link.HandleMountedStream:
		return c.handleMountedStream(ctx, di, d)
	case pubsub.BuildChannelSubscription:
		return c.resolveBuildChannelSub(ctx, di, d)
	}

	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		for _, ref := range c.cleanupRefs {
			ref.Release()
		}
		c.cleanupRefs = nil
		for k, l := range c.links {
			l.ctxCancel()
			delete(c.links, k)
		}
	})

	_ = c.pubSubCtr.SwapValue(func(val *pubsub.PubSub) *pubsub.PubSub {
		if val != nil {
			(*val).Close()
		}
		return nil
	})

	return nil
}

// _ is a type assertion
var _ pubsub.Controller = ((*Controller)(nil))
