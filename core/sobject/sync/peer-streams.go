package sobject_sync

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
)

// peerStreams owns the streams and renewals of one Execute call, keyed by the
// transport peer each stream runs with. PeerRetry.tla models its lifecycle.
//
// The solicit controller opens one stream for each pair of offers on a link,
// so a stream that ends leaves its link without another until one side offers
// the protocol again. A peer whose last stream ends recoverably renews only
// its own offer: a unique offer scoped to that peer, which pairs with the
// remote's standing offer and opens one new stream on that peer's links. The
// other peers' streams keep running. A stream this side refused waits until
// held authority admits the participant, then renews at once; a terminal end
// renews nothing and leaves the peer to the remote side's renewal.
type peerStreams struct {
	// s runs each stream and supplies held authority for readmission.
	s *SOSync
	// ctx is the owner's lifetime, shared by every stream and renewal.
	ctx context.Context
	// bcast guards stopping, workers and peers.
	bcast broadcast.Broadcast
	// stopping fences new streams and renewals while the owner drains.
	stopping bool
	// workers counts running stream and renewal goroutines.
	workers int
	// peers holds the lifecycle of each transport peer seen so far.
	peers map[peer.ID]*peerStream
}

// peerStream is the lifecycle of one transport peer.
type peerStream struct {
	// streams counts the running streams with the peer, one per link pair.
	streams int
	// renewal reopens the peer's stream; set only while streams is zero.
	renewal *peerRenewal
	// backoff spaces the peer's renewals, from syncRetryInitialInterval up to
	// syncRetryMaxInterval, for the owner's lifetime.
	backoff cbackoff.BackOff
}

// peerRenewal is one running renewal of a peer's offer.
type peerRenewal struct {
	// cancel stops the renewal goroutine.
	cancel context.CancelFunc
	// ref is the latest renewed offer, nil before the first. Whoever removes
	// the renewal from its peerStream releases it.
	ref directive.Reference
}

// newPeerStreams constructs the stream owner for one Execute lifetime.
func newPeerStreams(ctx context.Context, s *SOSync) *peerStreams {
	return &peerStreams{s: s, ctx: ctx, peers: make(map[peer.ID]*peerStream)}
}

// startStream runs a matched stream in its own goroutine, registered before it
// starts so shutdown cannot miss it. A stream matched after the owner began
// stopping is left unclaimed, and the solicit controller closes it.
func (o *peerStreams) startStream(sms link_solicit.SolicitMountedStream) {
	if o.addWorker() {
		go o.runStream(sms)
	}
}

// runStream claims a matched stream, runs it, and decides how the peer
// continues once it ends.
func (o *peerStreams) runStream(sms link_solicit.SolicitMountedStream) {
	// Release the goroutine's registration however the stream ends.
	defer o.workerDone()

	// Claim the stream once; a failed claim names no peer to renew.
	ms, taken, err := sms.AcceptMountedStream()
	if err != nil {
		o.s.le.WithError(err).Debug("matched shared object stream was not claimed")
		return
	}
	if taken {
		return
	}

	// Count the stream against its peer, ending any renewal for it.
	remote := ms.GetPeerID()
	local := ms.GetLink().GetLocalPeer()
	o.arrive(remote)

	// Run the authenticated stream with remote-scoped diagnostics.
	le := o.s.le.WithField("remote-peer", remote.String())
	err = o.s.runStream(o.ctx, le, ms.GetStream(), local, remote)
	if o.ctx.Err() == nil {
		o.s.logStreamResult(le, remote, err)
	}
	o.end(remote, local < remote, err)
}

// arrive counts a new stream with remote and removes the peer's renewal, which
// a stream from any offer makes unnecessary. Releasing the renewed offer
// refuses its other pairs; the accepted stream keeps running.
func (o *peerStreams) arrive(remote peer.ID) {
	// Take the renewal while counting the stream.
	var renewal *peerRenewal
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		p := o.peerLocked(remote)
		p.streams++
		renewal, p.renewal = p.renewal, nil
		bcast()
	})

	// Stop the renewal and withdraw its offer.
	if renewal != nil {
		renewal.stop()
	}
}

// end uncounts a stream with remote that returned err, and starts the peer's
// renewal when this was its last stream and err leaves a next step. lower
// reports whether the local transport peer ID sorts below remote's.
func (o *peerStreams) end(remote peer.ID, lower bool, err error) {
	// Classify the end: a refusal by held authority waits for readmission, a
	// terminal state waits for the remote side, and anything else retries.
	var refused *localRefusal
	isRefused := errors.As(err, &refused)
	renew := isRefused || !isTerminalSyncError(err)

	// Uncount the stream, and register the renewal only for the last one.
	var renewal *peerRenewal
	var wait time.Duration
	var rctx context.Context
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Uncount the stream; renew only after the peer's last one.
		p := o.peerLocked(remote)
		p.streams--
		bcast()
		if !renew || p.streams != 0 || o.stopping || o.ctx.Err() != nil {
			return
		}

		// Space recoverable retries. The side with the higher peer ID also
		// waits out the grace, so the lower side's renewal normally arrives
		// first and the two sides do not open a stream each.
		if !isRefused {
			wait = p.backoff.NextBackOff()
			if !lower {
				wait += syncRenewGrace
			}
		}
		var cancel context.CancelFunc
		rctx, cancel = context.WithCancel(o.ctx)
		renewal = &peerRenewal{cancel: cancel}
		p.renewal = renewal
		o.workers++
	})
	if renewal == nil {
		return
	}

	// Run the renewal until a stream with the peer arrives or the owner stops.
	go func() {
		defer o.workerDone()
		if isRefused {
			if err := o.s.waitReadmission(rctx, refused.remoteID); err != nil {
				if rctx.Err() != nil {
					return
				}
				// Without the watch, renew after a delay and let admission
				// decide on the new stream.
				o.s.le.WithError(err).WithField("remote-peer", remote.String()).
					Warn("failed to watch for readmission of refused participant")
				wait = syncRetryMaxInterval
			}
		}
		o.renew(rctx, remote, renewal, wait)
	}()
}

// renew waits for wait, then offers the protocol to remote under a new
// incarnation, and repeats with a slower backoff until rctx ends, since the
// solicit controller does not retry a failed open and the remote peer may have
// no link at all.
func (o *peerStreams) renew(rctx context.Context, remote peer.ID, renewal *peerRenewal, wait time.Duration) {
	le := o.s.le.WithField("remote-peer", remote.String())
	repeat := newSyncRenewBackoff()
	for {
		// Wait out the delay unless a stream arrives or the owner stops.
		if wait > 0 {
			le.WithField("retry-after", wait).Debug("renewing shared object synchronization with peer")
			select {
			case <-rctx.Done():
				return
			//gostyle:timer backoff
			case <-time.After(wait):
			}
		}

		// Offer the protocol to the peer and keep the newest offer.
		ref, err := o.s.addSolicitation(o.ctx, o, remote)
		if err != nil {
			le.WithError(err).Warn("failed to renew shared object synchronization with peer")
		} else if !o.replaceRenewal(remote, renewal, ref) {
			return
		}

		// Space the next renewal until a stream with the peer arrives.
		wait = repeat.NextBackOff()
	}
}

// replaceRenewal stores ref as the renewal's newest offer and releases the one
// it replaces. When the renewal was removed, by an arriving stream or by
// shutdown, it releases ref instead and returns false.
func (o *peerStreams) replaceRenewal(remote peer.ID, renewal *peerRenewal, ref directive.Reference) bool {
	// Swap the offer only while the renewal is still the peer's.
	var prev directive.Reference
	var current bool
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = !o.stopping && o.peers[remote].renewal == renewal
		if current {
			prev, renewal.ref = renewal.ref, ref
		}
	})

	// Release the offer that is no longer kept.
	if !current {
		ref.Release()
	} else if prev != nil {
		prev.Release()
	}
	return current
}

// stop fences new streams and renewals, cancels every stream and renewal
// through cancel, withdraws the renewed offers, and waits until every stream
// and renewal goroutine has returned. The caller releases the standing offer
// first, so unclaimed streams close instead of waiting on the join.
func (o *peerStreams) stop(cancel context.CancelFunc) {
	// Fence and take every renewal before canceling the shared context.
	var renewals []*peerRenewal
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		o.stopping = true
		for _, p := range o.peers {
			if p.renewal != nil {
				renewals = append(renewals, p.renewal)
				p.renewal = nil
			}
		}
		bcast()
	})
	cancel()
	for _, renewal := range renewals {
		renewal.stop()
	}

	// Wait on the same state owner until every goroutine has returned.
	for {
		var waitCh <-chan struct{}
		var stopped bool
		o.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			stopped = o.workers == 0
			waitCh = getWaitCh()
		})
		if stopped {
			return
		}
		<-waitCh
	}
}

// addWorker registers a stream goroutine unless the owner is stopping.
func (o *peerStreams) addWorker() bool {
	var added bool
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if o.stopping {
			return
		}
		o.workers++
		added = true
		bcast()
	})
	return added
}

// workerDone releases one stream or renewal goroutine.
func (o *peerStreams) workerDone() {
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		o.workers--
		bcast()
	})
}

// peerLocked returns the lifecycle of remote, creating it on first use.
// Caller must hold bcast.
func (o *peerStreams) peerLocked(remote peer.ID) *peerStream {
	p := o.peers[remote]
	if p == nil {
		p = &peerStream{backoff: newSyncRetryBackoff()}
		o.peers[remote] = p
	}
	return p
}

// stop cancels the renewal goroutine and withdraws its newest offer. The
// caller has removed the renewal from its peerStream, so no new offer is kept.
func (r *peerRenewal) stop() {
	r.cancel()
	if r.ref != nil {
		r.ref.Release()
	}
}
