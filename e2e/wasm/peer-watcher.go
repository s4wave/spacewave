//go:build !js

package wasm

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	devtool_web "github.com/s4wave/spacewave/bldr/devtool/web"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// PeerWatcher tracks browser peers discovered via HandleMountedStream
// directives on the devtool bus. It supports multi-session tests by
// sequencing peer observations and blocking until a browser peer connects.
type PeerWatcher struct {
	pending chan BrowserPeerObservation
	mu      sync.Mutex
	nextSeq uint64
	rel     func()
}

// BrowserPeerObservation describes one browser peer mount event seen by the
// devtool bus. A peer can reconnect with the same ID, so each observation has
// its own monotonic sequence number.
type BrowserPeerObservation struct {
	PeerID     peer.ID
	Sequence   uint64
	ObservedAt time.Time
}

// NewPeerWatcher registers a HandleMountedStream handler on the bus filtering
// for HostProtocolID and returns a PeerWatcher that tracks discovered peers.
func NewPeerWatcher(b bus.Bus) (*PeerWatcher, error) {
	pw := &PeerWatcher{
		pending: make(chan BrowserPeerObservation, 8),
	}
	rel, err := b.AddHandler(pw)
	if err != nil {
		return nil, errors.Wrap(err, "add peer watcher handler")
	}
	pw.rel = rel
	return pw, nil
}

// HandleDirective implements directive.Handler. It filters for
// HandleMountedStream directives on HostProtocolID and sends peer IDs to the
// pending channel. A peer can reconnect with the same ID after an early
// startup failure, so these events must not be deduplicated across the whole
// package run.
func (pw *PeerWatcher) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	hms, ok := di.GetDirective().(link.HandleMountedStream)
	if !ok {
		return nil, nil
	}
	if hms.HandleMountedStreamProtocolID() != devtool_web.HostProtocolID {
		return nil, nil
	}
	remotePeer := hms.HandleMountedStreamRemotePeerID()
	if len(remotePeer) == 0 {
		return nil, nil
	}

	pw.observePeer(remotePeer)
	return nil, nil
}

func (pw *PeerWatcher) observePeer(remotePeer peer.ID) {
	pw.mu.Lock()
	pw.nextSeq++
	obs := BrowserPeerObservation{
		PeerID:     remotePeer,
		Sequence:   pw.nextSeq,
		ObservedAt: time.Now(),
	}
	pw.mu.Unlock()

	select {
	case pw.pending <- obs:
	default:
		select {
		case <-pw.pending:
		default:
		}
		select {
		case pw.pending <- obs:
		default:
		}
	}
}

// WaitForNewPeer blocks until a browser peer mount event arrives and returns
// the most recent pending peer ID. Stale peer mount events can remain queued
// across subtest cleanup, so callers want the newest peer observation rather
// than the oldest buffered event.
func (pw *PeerWatcher) WaitForNewPeer(ctx context.Context) (peer.ID, error) {
	obs, err := pw.WaitForPeerObservation(ctx)
	if err != nil {
		return peer.ID(""), err
	}
	return obs.PeerID, nil
}

// WaitForPeerObservation blocks until a browser peer mount event arrives and
// returns the most recent pending observation. Stale peer mount events can
// remain queued across subtest cleanup, so callers want the newest peer
// observation rather than the oldest buffered event.
func (pw *PeerWatcher) WaitForPeerObservation(ctx context.Context) (BrowserPeerObservation, error) {
	var obs BrowserPeerObservation
	select {
	case obs = <-pw.pending:
	case <-ctx.Done():
		return BrowserPeerObservation{}, ctx.Err()
	}

	for {
		select {
		case obs = <-pw.pending:
		default:
			return obs, nil
		}
	}
}

// LatestSequence returns the latest peer observation sequence number. Callers
// can use it as a checkpoint before triggering a browser action.
func (pw *PeerWatcher) LatestSequence() uint64 {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	return pw.nextSeq
}

// WaitForPeerObservationAfter blocks until a browser peer mount event with a
// sequence greater than afterSeq arrives.
func (pw *PeerWatcher) WaitForPeerObservationAfter(
	ctx context.Context,
	afterSeq uint64,
) (BrowserPeerObservation, error) {
	for {
		obs, err := pw.WaitForPeerObservation(ctx)
		if err != nil {
			return BrowserPeerObservation{}, err
		}
		if obs.Sequence > afterSeq {
			return obs, nil
		}
	}
}

// Release removes the handler from the bus.
func (pw *PeerWatcher) Release() {
	if pw.rel != nil {
		pw.rel()
	}
}

// _ is a type assertion
var _ directive.Handler = (*PeerWatcher)(nil)
