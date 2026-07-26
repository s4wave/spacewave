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
	var lease *signalIngress
	var superseded bool
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
		var err error
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			current := w.incomingSessions[peerID]
			if lease == nil {
				lease, err = w.acquireSignalIngressLocked(peerID, resolver, broadcast)
				if err != nil {
					return
				}
				current = w.incomingSessions[peerID]
			} else {
				if current != nil && current.resolver != resolver {
					superseded = true
				}
				if current == nil && !superseded {
					lease, err = w.acquireSignalIngressLocked(peerID, resolver, broadcast)
					if err != nil {
						return
					}
					current = w.incomingSessions[peerID]
				}
			}
			ingress = current
			execution = w.snapshotSignalExecutionLocked(ingress)
			waitCh = getWaitCh()
		})
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
