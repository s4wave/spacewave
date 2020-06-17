package auth_challenge_client

import (
	"context"
	"sync"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"

	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "aperture/auth/challenge/client/1"

// Controller implements the auth challenge client.
//
// Connects to auth server peers on-demand.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// peerID is the local peer id
	peerID peer.ID
	// wakeCh wakes execute
	wakeCh chan struct{}

	mtx sync.Mutex
	// requests is the list of ongoing lookups
	// keyed by entity
	requests map[auth_challenge.RequestID]*lookupEntityRequest
	// remotePeers is the set of tracked remote auth peers
	remotePeers map[peer.ID]*authServerPeer
}

// NewController constructs a new auth challenge client.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) (*Controller, error) {
	pid, err := conf.ParsePeerID()
	if err != nil {
		return nil, err
	}
	remotePeerIDs, err := conf.ParseServerPeerIDs()
	if err != nil {
		return nil, err
	}
	c := &Controller{
		le:     le,
		bus:    bus,
		conf:   conf,
		peerID: pid,

		wakeCh:      make(chan struct{}, 1),
		requests:    make(map[auth_challenge.RequestID]*lookupEntityRequest),
		remotePeers: make(map[peer.ID]*authServerPeer),
	}
	for id := range remotePeerIDs {
		c.remotePeers[id] = newAuthServerPeer(c, id, conf)
	}
	return c, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.Info{
		Id:      ControllerID,
		Version: Version.String(),
	}
}

// AddRemotePeer adds a remote peer ID to track.
func (c *Controller) AddRemotePeer(id peer.ID) {
	c.mtx.Lock()
	_, ok := c.remotePeers[id]
	if !ok {
		c.remotePeers[id] = newAuthServerPeer(c, id, c.conf)
		defer c.wake()
	}
	c.mtx.Unlock()
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) (directive.Resolver, error) {
	dir := inst.GetDirective()
	switch d := dir.(type) {
	case identity.IdentityLookupEntity:
		return c.resolveLookupEntity(ctx, inst, d)
	}

	return nil, nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	wasEstablish := false
	establishCtx := ctx
	establishCtxCancel := func() {}

	for {
		select {
		case <-ctx.Done():
			establishCtxCancel()
			return ctx.Err()
		case <-c.wakeCh:
		}

		c.mtx.Lock()
		shouldEstablish := len(c.requests) > 0
		if shouldEstablish {
			var any bool
			for _, req := range c.requests {
				if req.result == nil {
					any = true
					break
				}
			}
			if !any {
				shouldEstablish = false
			}
		}

		// Establish conns to remote peers.
		if shouldEstablish != wasEstablish {
			if shouldEstablish {
				c.le.Info("starting connections to auth peers")
			} else {
				c.le.Info("closing connections to auth peers (became idle)")
			}
			wasEstablish = shouldEstablish
			establishCtxCancel()
			if shouldEstablish {
				establishCtx, establishCtxCancel = context.WithCancel(ctx)
			}
		}

		if shouldEstablish {
			for _, peer := range c.remotePeers {
				if peer.establishCtx != establishCtx {
					peer.establishCtx = establishCtx
					go peer.executeAuthClientSession(
						establishCtx,
						c,
					)
				}
			}
		}

		// Forward the requests set to the remote peers.
		for _, peerSess := range c.remotePeers {
			var wake bool
			for reqID, reqData := range c.requests {
				if reqData.result != nil {
					continue
				}
				if _, ok := peerSess.txRequests[reqID]; ok {
					continue
				}
				domainID, entityID := reqData.domainID, reqData.entityID
				peerSess.txRequests[reqID] = struct{}{}
				peerSess.txQueue = append(peerSess.txQueue, &auth_challenge.Msg{
					MsgType: auth_challenge.MsgType_MsgType_ENTITY_LOOKUP_START,
					EntityLookupStart: auth_challenge.NewEntityLookupStart(
						domainID, entityID,
					),
				})
				wake = true
			}

			// Scan for canceled requests.
			// If the request no longer exists, tx a cancel msg.
			for rid := range peerSess.txRequests {
				if req, ok := c.requests[rid]; !ok || req.result != nil {
					delete(peerSess.txRequests, rid)
					peerSess.enqueueMsg(&auth_challenge.Msg{
						MsgType: auth_challenge.MsgType_MsgType_ENTITY_LOOKUP_CANCEL,
						EntityLookupCancel: auth_challenge.NewEntityLookupCancel(
							rid[0],
							rid[1],
						),
					})
					wake = true
				}
			}

			if wake {
				peerSess.wake()
			}
		}

		c.mtx.Unlock()
	}
}

// getOrAddLookup gets or adds a lookup handle, adding refcount.
func (c *Controller) getOrAddLookup(domainID, entityID string, cb lookupEntityResultCb) (*lookupEntityRequest, int) {
	c.mtx.Lock()
	rid := auth_challenge.NewRequestID(domainID, entityID)
	ex, ok := c.requests[rid]
	var refID int
	if ok {
		refID = ex.addRefcount(cb)
	} else {
		// recount starts at 1
		ex = newLookupEntityRequest(domainID, entityID, cb)
		c.requests[rid] = ex
		// refID == 0
	}
	c.mtx.Unlock()
	return ex, refID
}

// releaseLookup releases a lookup handle (call exactly once after getOrAdd)
// expects mtx is locked
func (c *Controller) releaseLookup(h *lookupEntityRequest, refID int) {
	c.mtx.Lock()
	h.decRefcount(refID)
	if len(h.cbs) == 0 {
		rid := h.rid
		ex, ok := c.requests[rid]
		if ok && ex == h {
			delete(c.requests, rid)
		}
	}
	c.mtx.Unlock()
}

// wake wakes up the execute loop.
func (c *Controller) wake() {
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
