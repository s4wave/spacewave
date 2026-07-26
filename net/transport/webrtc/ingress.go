package webrtc

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
)

// signalIngress owns the one keyed lease that keeps a peer's signal tracker live.
type signalIngress struct {
	resolver *handleSignalPeerResolver
	ref      *keyed.KeyedRef[string, *sessionTracker]
	tracker  *sessionTracker
}

// acquireSignalIngressLocked acquires or replaces the peer's ingress lease.
// The caller must hold w.bcast.
func (w *WebRTC) acquireSignalIngressLocked(
	peerID string,
	resolver *handleSignalPeerResolver,
	broadcast func(),
) (*signalIngress, error) {
	current := w.incomingSessions[peerID]
	if current != nil && current.resolver == resolver {
		return current, nil
	}

	ref, tracker, _, err := w.addSessionTrackerRef(peerID)
	if err != nil {
		return nil, err
	}

	next := &signalIngress{
		resolver: resolver,
		ref:      ref,
		tracker:  tracker,
	}
	w.incomingSessions[peerID] = next
	if current != nil {
		current.ref.Release()
	}
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

// closeSignalIngress closes the current lease for a resolver.
func (w *WebRTC) closeSignalIngress(peerID string, resolver *handleSignalPeerResolver) {
	w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress := w.incomingSessions[peerID]
		if ingress == nil || ingress.resolver != resolver {
			return
		}
		delete(w.incomingSessions, peerID)
		ingress.ref.Release()
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
			// A resolver that has never held the lease supersedes the
			// current holder: a fresh inbound signal session replaces the
			// previous one. A resolver that held the lease and was
			// superseded reacquires only when the peer has no lease at
			// all, including after the superseding ingress retires, so it
			// never steals the lease from a live successor; otherwise
			// delivery targets the successor's live execution. The held
			// history lives on the resolver so every later signal from a
			// superseded session observes it.
			current := w.incomingSessions[peerID]
			if current == nil || (current.resolver != resolver && !resolver.held) {
				if _, err = w.acquireSignalIngressLocked(peerID, resolver, broadcast); err != nil {
					return
				}
				current = w.incomingSessions[peerID]
			}
			if current.resolver == resolver {
				resolver.held = true
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
