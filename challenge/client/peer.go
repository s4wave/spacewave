package auth_challenge_client

import (
	"context"
	"time"

	"github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/bifrost/link"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/stream"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// authServerPeer contains information about a remote auth server peer.
type authServerPeer struct {
	c      *Controller
	id     peer.ID
	bo     backoff.BackOff
	wakeCh chan struct{}

	// below guarded by mtx on controller
	establishCtx context.Context
	// txRequests are requests that have been queued to transmit
	txRequests map[auth_challenge.RequestID]struct{}
	// txQueue are messages that are pending to be sent
	txQueue []*auth_challenge.Msg
}

// newAuthServerPeer constructs a new authServerPeer.
func newAuthServerPeer(c *Controller, id peer.ID, conf *Config) *authServerPeer {
	return &authServerPeer{
		c:          c,
		id:         id,
		bo:         conf.GetPerServerBackoff().Construct(),
		wakeCh:     make(chan struct{}, 1),
		txRequests: make(map[auth_challenge.RequestID]struct{}),
	}
}

// enqueueMsg enqueues a message.
// expects controller mtx to be locked
// call wake afterwards to transmit
func (p *authServerPeer) enqueueMsg(msg *auth_challenge.Msg) {
	p.txQueue = append(p.txQueue, msg)
}

// dequeueMsg dequeues a message.
// expects controller mtx to be locked
func (p *authServerPeer) dequeueMsg() *auth_challenge.Msg {
	if len(p.txQueue) == 0 {
		return nil
	}
	v := p.txQueue[0]
	p.txQueue = p.txQueue[1:]
	if len(p.txQueue) == 0 {
		p.txQueue = nil
	}
	return v
}

// wake indicates that a new request has been added.
func (p *authServerPeer) wake() {
	select {
	case p.wakeCh <- struct{}{}:
	default:
	}
}

// executeAuthClientSession executes the auth client session.
func (p *authServerPeer) executeAuthClientSession(
	ctx context.Context,
	c *Controller,
) {
	le := c.le
	b := c.bus
	conf := c.conf
	localPeerID := c.peerID
	logger := func() *logrus.Entry {
		return le.WithField("remote-peer-id", p.id.Pretty())
	}
	_ = logger
	_ = conf
	_ = localPeerID

	// add a continuous establish link directive
	_, estRef, err := b.AddDirective(link.NewEstablishLinkWithPeer(p.id), nil)
	if err != nil {
		logger().WithError(err).Warn("cannot establish link")
		return
	}
	defer estRef.Release()

	bo := p.bo
	for {
		attemptCtx, attemptCtxCancel := context.WithCancel(ctx)
		attemptErr := p.executeAuthClientSessionOnce(attemptCtx, le, b)
		attemptCtxCancel()
		if attemptErr == nil {
			bo.Reset()
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
			if attemptErr == context.Canceled || attemptErr == context.DeadlineExceeded {
				return
			}
		}
		p.c.mtx.Lock()
		p.txQueue = nil
		for i := range p.txRequests {
			delete(p.txRequests, i)
		}
		p.c.mtx.Unlock()
		nextBo := bo.NextBackOff()
		logger().
			WithError(attemptErr).
			WithField("backoff", nextBo.String()).
			Warn("error attempting auth session")
		boTimer := time.NewTimer(nextBo)
		select {
		case <-ctx.Done():
			boTimer.Stop()
			return
		case <-boTimer.C:
			// try again
			p.c.wake()
			continue
		}
	}
}

// executeAuthClientSessionOnce executes the auth client session once.
func (p *authServerPeer) executeAuthClientSessionOnce(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
) error {
	mstrm, rel, err := link.OpenStreamWithPeerEx(
		ctx,
		b,
		auth_challenge.ChallengeProtocolID,
		peer.ID(""), p.id, 0,
		stream.OpenOpts{Reliable: true, Encrypted: true},
	)
	if err != nil {
		return err
	}
	defer rel()

	linkUUID := mstrm.GetLink().GetUUID()
	le.
		WithField("link-uuid", linkUUID).
		Debug("established link and stream")
	if !mstrm.GetOpenOpts().Encrypted {
		return errors.New("stream was not encrypted but must be for auth session")
	}

	// while session is running, transmit all requests + service replies
	// when woken, scan the list to see if any new requests are added / removed
	strm := mstrm.GetStream()
	sess := auth_challenge.NewSession(strm)
	defer strm.Close()
	// start reader goroutine
	errCh := make(chan error, 1)
	msgCh := make(chan *auth_challenge.Msg, 5)
	go func() {
		errCh <- func() error {
			m := &auth_challenge.Msg{}
			for {
				err := sess.ReadMsg(m)
				if err != nil {
					return err
				}

				// backpressure a bit here to avoid DoS
				select {
				case <-ctx.Done():
					return context.Canceled
				case msgCh <- m:
					m = &auth_challenge.Msg{}
				}
			}
		}()
	}()

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case m := <-msgCh:
			mle := le.
				WithField("msg-type", m.GetMsgType().String()).
				WithField("remote-peer", mstrm.GetPeerID().Pretty())
			if err := p.handleIncomingMessage(m); err != nil {
				if err == context.Canceled {
					continue
				}
				mle.
					WithError(err).
					Warn("error handling incoming message")
			} else {
				mle.Info("got incoming message")
			}
			continue
		case <-p.wakeCh:
		}

		// transmit any outgoing messages
		for {
			p.c.mtx.Lock()
			outMsg := p.dequeueMsg()
			p.c.mtx.Unlock()
			if outMsg == nil {
				break
			}

			if err := sess.SendMsg(outMsg); err != nil {
				return err
			}
		}
	}
}

// handleIncomingMessage handles an incoming message.
func (p *authServerPeer) handleIncomingMessage(m *auth_challenge.Msg) error {
	switch m.GetMsgType() {
	case auth_challenge.MsgType_MsgType_ENTITY_LOOKUP_FINISH:
		fin := m.GetEntityLookupFinish()
		rid := fin.GetIdentifier().ToRequestID()
		p.c.mtx.Lock()
		if _, ok := p.txRequests[rid]; !ok {
			p.c.mtx.Unlock()
			return nil
		}

		req, reqOk := p.c.requests[rid]
		if reqOk {
			// this will transmit a cancel to all other peers that have seen the req
			delete(p.txRequests, rid)
			req.applyResult(fin) // trigger callbacks
			defer p.c.wake()
		}
		p.c.mtx.Unlock()
		return nil
	default:
		return errors.Errorf("unexpected message type: %s", m.GetMsgType().String())
	}
}
