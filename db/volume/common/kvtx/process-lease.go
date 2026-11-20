package kvtx

import "sync"

// processLease names the leases under which this process holds its roots, as
// block_gc's leases.go describes. Each ID is a new ULID, so a process that
// opens the Volume again, or whose lease lapsed while it stalled, never holds
// a root under a lease that lapsed.
//
// A changing transaction body sets these fields before its commit. A failed
// commit leaves an ID whose record the store lacks, which the next writer
// reads as not live, so the next renewal or fill starts a lease again.
type processLease struct {
	// mu guards the fields below.
	mu sync.Mutex
	// id is the lease of this process's reader pins and stage roots, empty
	// before the first. Only renewRoots sets it, moving every root this
	// process holds onto it in the same transaction.
	id string
	// fill is the lease of the roots of this process's cache fills, empty
	// before the first fill. It is never renewed: a fill opens a new one
	// once fewer than the hold is left of it.
	fill string
	// fillExpires is the expiry of fill in Unix milliseconds.
	fillExpires int64
}
