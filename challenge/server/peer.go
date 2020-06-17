package auth_challenge_server

import (
	"time"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/cenkalti/backoff"
)

// authClientPeer contains information about a remote auth client peer.
type authClientPeer struct {
	c            *Controller
	id           peer.ID
	bo           backoff.BackOff
	backOffUntil time.Time

	// below guarded by mtx on controller
	// requests contains ongoing requests
	requests map[auth_challenge.RequestID]*remoteEntityLookupRequest
	// sessions contains ongoing sessions
	sessions map[*streamHandler]struct{}
}

// newAuthClientPeer constructs a new authClientPeer.
func newAuthClientPeer(c *Controller, id peer.ID, conf *Config, initSess *streamHandler) *authClientPeer {
	return &authClientPeer{
		c:  c,
		id: id,
		bo: conf.GetPerClientBackoff().Construct(),

		requests: make(map[auth_challenge.RequestID]*remoteEntityLookupRequest),
		sessions: map[*streamHandler]struct{}{
			initSess: struct{}{},
		},
	}
}

// addRefcount adds a reference to the request.
// expects mtx to be locked on controller
func (p *authClientPeer) addRefcount(sess *streamHandler) {
	p.sessions[sess] = struct{}{}
}

// decRefcount decrements the refcount.
// expects mtx to be locked on controller
func (p *authClientPeer) decRefcount(sess *streamHandler) {
	delete(p.sessions, sess)
}

// HandleMsg handles an incoming message.
// msg should not be reused
func (p *authClientPeer) HandleMsg(msg *auth_challenge.Msg) error {
	le := p.c.le.WithField("peer-id", p.id.Pretty())
	le.Debugf("handling message with type %s", msg.GetMsgType().String())
	switch msg.GetMsgType() {
	case auth_challenge.MsgType_MsgType_ENTITY_LOOKUP_CANCEL:
		rid := msg.GetEntityLookupCancel().GetIdentifier().ToRequestID()
		p.c.mtx.Lock()
		// find the request and cancel it
		req, reqOk := p.requests[rid]
		if reqOk {
			if req.ctxCancel != nil {
				req.ctxCancel()
				req.ctxCancel = nil
			}
			delete(p.requests, rid)
		}
		p.c.mtx.Unlock()
		return nil
	case auth_challenge.MsgType_MsgType_ENTITY_LOOKUP_START:
		rid := msg.GetEntityLookupStart().GetIdentifier().ToRequestID()
		p.c.mtx.Lock()
		// start the request
		_, reqOk := p.requests[rid]
		if !reqOk {
			nreq := newRemoteEntityLookupRequest(p, rid)
			p.requests[rid] = nreq
			p.c.startRequestQueue = append(p.c.startRequestQueue, nreq)
			defer p.c.wake()
		}
		p.c.mtx.Unlock()
		return nil
	default:
		le.
			WithField("msg-type", msg.GetMsgType().String()).
			Warn("dropping unknown message type")
		return nil
	}
}
