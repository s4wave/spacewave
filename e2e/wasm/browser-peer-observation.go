//go:build !js

package wasm

import (
	"time"

	"github.com/s4wave/spacewave/net/peer"
)

// BrowserPeerObservation records a browser peer mounting the devtool stream.
type BrowserPeerObservation struct {
	// PeerID identifies the browser peer.
	PeerID peer.ID
	// Sequence increases for each mount, including reconnects from the same peer.
	Sequence uint64
	// ObservedAt records when the watcher saw the mount.
	ObservedAt time.Time
}
