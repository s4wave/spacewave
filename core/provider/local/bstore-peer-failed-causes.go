package provider_local

// PeerFailedCauses counts the failed peer reads by why they failed. A read
// that asked more than once counts by the last cause it found, other than a
// timeout, which counts only when no attempt found another cause.
type PeerFailedCauses struct {
	// PeerError counts reads a peer answered with an error, or whose exchange
	// with a peer failed.
	PeerError uint64
	// Timeout counts reads whose bound ended with a request outstanding.
	Timeout uint64
	// NoSession counts reads that found no open peer session to ask, or whose
	// session closed under the request.
	NoSession uint64
}
