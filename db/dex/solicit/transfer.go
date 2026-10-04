package dex_solicit

import "time"

// TransferSnapshot reports payload traffic and current peers for this
// controller. Counters exclude cached reads and protocol framing and last for
// its lifetime.
type TransferSnapshot struct {
	// UploadedBytes counts block payload bytes sent to peers.
	UploadedBytes uint64
	// DownloadedBytes counts block payload bytes received from peers.
	DownloadedBytes uint64
	// LastActivity is when a payload last crossed a peer stream.
	LastActivity time.Time
	// Peers lists every peer with traffic or a current session.
	Peers []PeerTransferSnapshot
}
