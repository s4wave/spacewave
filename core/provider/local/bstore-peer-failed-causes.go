package provider_local

// PeerFailedCauses counts the failed peer reads by why they failed. A read
// that asked more than once counts by the last cause it found, other than a
// timeout, which counts only when no attempt found another cause.
type PeerFailedCauses struct {
	// PeerError counts reads a peer answered with an error: its own lookup of
	// the block failed.
	PeerError uint64
	// Timeout counts reads whose bound ended with a request outstanding.
	Timeout uint64
	// NoSession counts reads that found no open peer session to ask, or whose
	// session closed under the request.
	NoSession uint64
	// Exchange counts reads whose exchange with a peer failed, such as a
	// failed send, a reset stream or a block that failed to verify.
	Exchange uint64
	// Denied counts reads a peer rejected for the requester's read authority.
	Denied uint64
}
