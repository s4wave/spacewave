package resource_state

import "context"

// StateAtomStore is the interface for state atom storage backends.
type StateAtomStore interface {
	// GetStoreID returns the unique identifier for this store.
	GetStoreID() string

	// Get returns the current state JSON and sequence number.
	Get(ctx context.Context) (stateJson string, seqno uint64, err error)

	// Set updates the state JSON and returns the new sequence number.
	Set(ctx context.Context, stateJson string) (seqno uint64, err error)

	// WaitSeqno blocks until the seqno is >= the given value.
	WaitSeqno(ctx context.Context, minSeqno uint64) (uint64, error)
}
