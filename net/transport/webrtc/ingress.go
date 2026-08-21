package webrtc

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
)

// signalIngress owns the one keyed lease that keeps a peer's signal tracker live.
type signalIngress struct {
	resolvers map[*handleSignalPeerResolver]struct{}
	ref       *keyed.KeyedRef[string, *sessionTracker]
	tracker   *sessionTracker
}

// acquireSignalIngressLocked acquires the peer's ingress lease.
// A live lease remains authoritative until its tracker execution retires.
// The caller must hold w.bcast.
func (w *WebRTC) acquireSignalIngressLocked(
	peerID string,
	resolver *handleSignalPeerResolver,
	broadcast func(),
) (*signalIngress, error) {
	if resolver.closed {
		return nil, context.Canceled
	}

	current := w.incomingSessions[peerID]
	if current != nil {
		if _, member := current.resolvers[resolver]; !member {
			current.resolvers[resolver] = struct{}{}
			broadcast()
		}
		return current, nil
	}

	ref, tracker, _, err := w.addSessionTrackerRef(peerID)
	if err != nil {
		return nil, err
	}

	next := &signalIngress{
		resolvers: map[*handleSignalPeerResolver]struct{}{resolver: {}},
		ref:       ref,
		tracker:   tracker,
	}
	w.incomingSessions[peerID] = next
	broadcast()
	return next, nil
}

// snapshotSignalExecutionLocked returns the current live execution.
// The caller must hold w.bcast.
func (w *WebRTC) snapshotSignalExecutionLocked(ingress *signalIngress) *sessionTrackerExecution {
	if ingress == nil || ingress.tracker == nil {
		return nil
	}
	return ingress.tracker.execution
}

// retireSignalIngressLocked retires the lease for a completed execution.
// The caller must hold w.bcast.
func (w *WebRTC) retireSignalIngressLocked(peerID string, tracker *sessionTracker) {
	ingress := w.incomingSessions[peerID]
	if ingress == nil || ingress.tracker != tracker {
		return
	}
	delete(w.incomingSessions, peerID)
	ingress.ref.Release()
}

// closeSignalIngress removes a resolver from the peer's ingress lease.
func (w *WebRTC) closeSignalIngress(peerID string, resolver *handleSignalPeerResolver) {
	w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		resolver.closed = true
		ingress := w.incomingSessions[peerID]
		if ingress == nil {
			return
		}
		if _, ok := ingress.resolvers[resolver]; !ok {
			return
		}
		delete(ingress.resolvers, resolver)
		if len(ingress.resolvers) == 0 {
			delete(w.incomingSessions, peerID)
			ingress.ref.Release()
		}
		broadcast()
	})
}

// deliverSignal submits a decoded signal to the live execution at most once.
func (w *WebRTC) deliverSignal(
	ctx context.Context,
	peerID string,
	resolver *handleSignalPeerResolver,
	incoming *incomingSignal,
) error {
	var deliveredTracker *sessionTracker
	var deliveredGeneration uint64

	for {
		select {
		case <-incoming.accepted:
			return nil
		default:
		}

		var ingress *signalIngress
		var execution *sessionTrackerExecution
		var waitCh <-chan struct{}
		var accepted bool
		var err error
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// Recheck acceptance under the lock: a signal accepted between
			// the unlocked check above and lock entry must not acquire a
			// lease it will never use.
			select {
			case <-incoming.accepted:
				accepted = true
				return
			default:
			}
			// All signaling sessions for one peer deliver to the same live
			// tracker. Replacing its ingress lease here would cancel an
			// active negotiation before its answer or ICE arrives.
			current, acquireErr := w.acquireSignalIngressLocked(peerID, resolver, broadcast)
			if acquireErr != nil {
				err = acquireErr
				return
			}
			ingress = current
			execution = w.snapshotSignalExecutionLocked(ingress)
			waitCh = getWaitCh()
		})
		if accepted {
			return nil
		}
		if err != nil {
			return err
		}

		if execution == nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-incoming.accepted:
				return nil
			case <-waitCh:
				continue
			}
		}

		if deliveredTracker != ingress.tracker || deliveredGeneration != execution.generation {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-incoming.accepted:
				return nil
			case <-waitCh:
				continue
			case execution.rxSignal <- incoming:
				deliveredTracker = ingress.tracker
				deliveredGeneration = execution.generation
			}
			continue
		}

		select {
		case <-ctx.Done():
			return context.Canceled
		case <-incoming.accepted:
			return nil
		case <-waitCh:
			continue
		}
	}
}
