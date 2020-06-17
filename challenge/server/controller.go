package auth_challenge_server

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/bifrost/link"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"

	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "aperture/auth/challenge/server/1"

// Controller implements the auth challenge server.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// peerID is the peer id
	peerID peer.ID
	// wakeCh wakes execute
	wakeCh chan struct{}

	// mtx guards below fields
	mtx sync.Mutex
	// peerSessions contains tracked remote peers
	peerSessions map[peer.ID]*authClientPeer
	// startRequestQueue is the start request fifo queue
	startRequestQueue []*remoteEntityLookupRequest
}

// NewController constructs a new auth challenge server.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) (*Controller, error) {
	pid, err := conf.ParsePeerID()
	if err != nil {
		return nil, err
	}

	return &Controller{
		le:     le,
		bus:    bus,
		conf:   conf,
		peerID: pid,
		wakeCh: make(chan struct{}, 1),

		peerSessions: make(map[peer.ID]*authClientPeer),
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.Info{
		Id:          ControllerID,
		Version:     Version.String(),
		Description: "auth server for peer " + c.conf.GetPeerId(),
	}
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
	case link.HandleMountedStream:
		return c.resolveHandleMountedStream(ctx, inst, d)
	}

	return nil, nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Remember remote peers at least until backOffUntil.
	var latestC <-chan time.Time
	var latestTimer *time.Timer
	var latestTimerProc time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.wakeCh:
		case <-latestC:
		}

		// clear timer
		if latestTimer != nil {
			if !latestTimer.Stop() {
				select {
				case <-latestTimer.C:
				default:
				}
			}
			latestTimer = nil
			latestC = nil
		}

		c.mtx.Lock()
		if len(c.peerSessions) == 0 {
			c.startRequestQueue = nil
			c.mtx.Unlock()
			continue
		}

		now := time.Now()
		latestTimerProc = now

		// scan remote peers, determine next time we will flush something
		for peerID, peer := range c.peerSessions {
			if len(peer.sessions) != 0 {
				continue
			}
			if peer.backOffUntil.After(now) {
				if peer.backOffUntil.After(latestTimerProc) {
					latestTimerProc = peer.backOffUntil
				}
			} else {
				delete(c.peerSessions, peerID)
			}
		}

		// start requests
		for _, req := range c.startRequestQueue {
			if req.ctxCancel != nil || len(req.peer.sessions) == 0 {
				continue
			}
			if req.peer.requests[req.rid] != req {
				continue
			}
			req.ctx, req.ctxCancel = context.WithCancel(ctx)
			go req.executeEntityLookupRequest()
		}
		c.startRequestQueue = nil

		// flush wakeCh
		select {
		case <-c.wakeCh:
		default:
		}
		c.mtx.Unlock()

		// setup next timer
		if latestTimerProc.After(now) {
			durUntil := latestTimerProc.Sub(now)
			latestTimer = time.NewTimer(durUntil)
			latestC = latestTimer.C
		}
	}
}

// addPeerReference adds a reference to a peer session.
func (c *Controller) addPeerReference(peerID peer.ID, handler *streamHandler) *authClientPeer {
	c.mtx.Lock()
	ex, ok := c.peerSessions[peerID]
	if !ok {
		ex = newAuthClientPeer(c, peerID, c.conf, handler)
		c.peerSessions[peerID] = ex
	} else {
		ex.addRefcount(handler)
	}
	c.mtx.Unlock()
	return ex
}

// releasePeerRef releases a peer reference.
func (c *Controller) releasePeerReference(ref *authClientPeer, sess *streamHandler) {
	peerID := ref.id
	c.mtx.Lock()
	if c.peerSessions[peerID] == ref {
		ref.decRefcount(sess)
		now := time.Now()
		if len(ref.sessions) == 0 {
			if ref.backOffUntil.Before(now) {
				delete(c.peerSessions, peerID)
			} else {
				defer c.wake()
			}
			for reqid, req := range ref.requests {
				if req.ctxCancel != nil {
					req.ctxCancel()
					req.ctxCancel = nil
				}
				delete(ref.requests, reqid)
			}
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
