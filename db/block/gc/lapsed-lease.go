package block_gc

// LapsedLease is a lease whose expiry passed, or which the collector released.
type LapsedLease struct {
	// ID is the lease.
	ID string
	// Expires is its expiry in Unix milliseconds, zero once released.
	Expires int64
}
