//go:build !js

package wasm

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	devtool_web "github.com/s4wave/spacewave/bldr/devtool/web"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// PeerWatcher tracks browser peers discovered via HandleMountedStream
// directives on the devtool bus. It supports multi-session tests by
// tracking seen peers and blocking until a previously unseen peer connects.
type PeerWatcher struct {
	pending chan peer.ID
	rel     func()
}

// NewPeerWatcher registers a HandleMountedStream handler on the bus filtering
// for HostProtocolID and returns a PeerWatcher that tracks discovered peers.
func NewPeerWatcher(b bus.Bus) (*PeerWatcher, error) {
	pw := &PeerWatcher{
		pending: make(chan peer.ID, 8),
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

	select {
	case pw.pending <- remotePeer:
	default:
	}
	return nil, nil
}

// WaitForNewPeer blocks until a browser peer mount event arrives and returns
// the most recent pending peer ID. Stale peer mount events can remain queued
// across subtest cleanup, so callers want the newest peer observation rather
// than the oldest buffered event.
func (pw *PeerWatcher) WaitForNewPeer(ctx context.Context) (peer.ID, error) {
	var p peer.ID
	select {
	case p = <-pw.pending:
	case <-ctx.Done():
		return peer.ID(""), ctx.Err()
	}

	for {
		select {
		case p = <-pw.pending:
		default:
			return p, nil
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
