package webrtc

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
)

// signalIngress owns the one keyed lease that keeps a peer's signal tracker live.
//
// The ingress outlives tracker generations: retiring an execution detaches the
// tracker while resolvers remain, so material admitted by the retired
// generation stays attributable to this ingress instead of silently rebinding
// to its successor.
type signalIngress struct {
	resolvers map[*handleSignalPeerResolver]struct{}
	// ref and tracker are nil while the ingress has no live tracker
	// generation attached.
	ref     *keyed.KeyedRef[string, *sessionTracker]
	tracker *sessionTracker

	// fencedEra reports that this ingress admitted an SDP signal carrying no
	// offer identity. Every later SDP or ICE signal from this peer belongs
	// to an era this transport cannot attribute, so they are dropped until
	// the ingress closes. request_offer markers stay exempt: they carry no
	// generation material and wake the renegotiation that re-tags traffic.
	fencedEra bool
}

// acquireSignalIngressLocked acquires the peer's ingress lease.
// A live lease remains authoritative across tracker regeneration: a successor
// tracker rebinds onto the existing ingress under this lock.
// The caller must hold w.bcast.
func (w *WebRTC) acquireSignalIngressLocked(
	peerID string,
	resolver *handleSignalPeerResolver,
	broadcast func(),
) (*signalIngress, error) {
	if resolver.closed {
		return nil, context.Canceled
	}

	if current := w.incomingSessions[peerID]; current != nil {
		_, member := current.resolvers[resolver]
		if current.tracker == nil {
			// The previous tracker generation retired; acquire its successor.
			ref, tracker, _, err := w.addSessionTrackerRef(peerID)
			if err != nil {
				return nil, err
			}
			current.ref = ref
			current.tracker = tracker
			broadcast()
		}
		if !member {
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

// retireSignalIngressLocked detaches a retired tracker execution from the
// peer's ingress lease. The ingress stays discoverable until its last resolver
// closes so parked deliveries remain attributable to it; the next acquisition
// rebinds a successor tracker under the hold lock. The caller must hold
// w.bcast.
func (w *WebRTC) retireSignalIngressLocked(peerID string, tracker *sessionTracker, broadcast func()) {
	ingress := w.incomingSessions[peerID]
	if ingress == nil || ingress.tracker != tracker {
		return
	}
	ingress.tracker = nil
	ingress.ref.Release()
	ingress.ref = nil
	if len(ingress.resolvers) == 0 {
		delete(w.incomingSessions, peerID)
	}
	broadcast()
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
			if ingress.ref != nil {
				ingress.ref.Release()
				ingress.ref = nil
			}
		}
		broadcast()
	})
}

// isGenerationFencedBody reports whether a signal body carries offer
// generation material that must never outlive the tracker execution it was
// admitted to. request_offer markers are exempt: they carry no generation
// material and act as the bounded positive wake for renegotiation.
func isGenerationFencedBody(sig *WebRtcSignal) bool {
	switch sig.GetBody().(type) {
	case *WebRtcSignal_Sdp, *WebRtcSignal_Ice:
		return true
	default:
		return false
	}
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
	// admitTracker and admitGeneration record the first live generation this
	// delivery was parked against. If that generation retires or is replaced,
	// fenced material is dropped instead of replaying into a successor.
	var admitTracker *sessionTracker
	var admitGeneration uint64

	for {
		select {
		case <-incoming.accepted:
			return nil
		default:
		}

		var ingress *signalIngress
		var execution *sessionTrackerExecution
		var tracker *sessionTracker
		var waitCh <-chan struct{}
		var accepted bool
		var fenced bool
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

			// Admission fence: negotiation material that carries no offer
			// id cannot be attributed to any generation. Once one untagged
			// description is seen, the peer's whole signal era is fenced,
			// not just that one message. request_offer stays exempt.
			switch b := incoming.sig.GetBody().(type) {
			case *WebRtcSignal_Sdp:
				if len(b.Sdp.GetOfferId()) == 0 && b.Sdp.GetSdpType() != "" {
					// A real description with no offer identity cannot be
					// attributed to any generation.
					ingress.fencedEra = true
					fenced = true
				}
			case *WebRtcSignal_Ice:
				if len(b.Ice.GetOfferId()) == 0 && ingress.fencedEra {
					fenced = true
				}
			}

			execution = w.snapshotSignalExecutionLocked(ingress)
			// Snapshot the tracker under the same lock: retirement and
			// acquisition mutate it concurrently, so the admit and deliver
			// decisions below must read the same coherent value.
			tracker = ingress.tracker
			waitCh = getWaitCh()
		})
		if accepted || fenced {
			if fenced {
				w.le.Debug("dropping signal: empty offer id in a fenced era")
			}
			return nil
		}
		if err != nil {
			return err
		}

		if execution == nil {
			if admitTracker != nil && isGenerationFencedBody(incoming.sig) {
				w.le.Debug("dropping stale-generation signal: tracker execution retired")
				return nil
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

		if admitTracker == nil {
			admitTracker = tracker
			admitGeneration = execution.generation
		} else if admitTracker != tracker || admitGeneration != execution.generation {
			if isGenerationFencedBody(incoming.sig) {
				w.le.Debug("dropping stale-generation signal: superseded by successor tracker")
				return nil
			}
			admitTracker = tracker
			admitGeneration = execution.generation
		}

		if deliveredTracker != tracker || deliveredGeneration != execution.generation {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-incoming.accepted:
				return nil
			case <-waitCh:
				continue
			case execution.rxSignal <- incoming:
				deliveredTracker = tracker
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
