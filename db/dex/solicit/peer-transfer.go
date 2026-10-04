package dex_solicit

// PeerTransferSnapshot retains this peer's traffic across stream reconnects.
type PeerTransferSnapshot struct {
	// PeerID is the remote peer id.
	PeerID string
	// UploadedBytes counts block payload bytes sent to this peer.
	UploadedBytes uint64
	// DownloadedBytes counts block payload bytes received from this peer.
	DownloadedBytes uint64
	// Connected reports whether the peer has a current session.
	Connected bool
}
