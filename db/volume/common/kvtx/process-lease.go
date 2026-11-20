package kvtx

import (
	"sync"

	"github.com/aperturerobotics/util/csync"
)

// processLease names the leases under which this process holds its roots, as
// block_gc's leases.go describes. Each ID is a new ULID, so a process that
// opens the Volume again, or whose lease lapsed while it stalled, never holds
// a root under a lease that lapsed.
//
// A fill's transaction body sets fill before its commit; a failed commit
// leaves an ID whose record the store lacks, which the next fill reads as not
// live and replaces. renewRoots sets id only after its commit.
type processLease struct {
	// renew runs one renewRoots at a time, from its snapshot of the held
	// roots through its commit to the publication of id, so a second renewal
	// reads the lease the first started and renews it instead of starting
	// another and leaving the first's roots to lapse. It is taken outside any
	// transaction and before rootPinMu.
	renew csync.Mutex
	// mu guards the fields below.
	mu sync.Mutex
	// id is the lease of this process's reader pins and stage roots, empty
	// before the first. Only renewRoots sets it, under renew, moving every
	// root this process holds onto it in the same transaction.
	id string
	// fill is the lease of the roots of this process's cache fills, empty
	// before the first fill. It is never renewed: a fill opens a new one
	// once fewer than the hold is left of it.
	fill string
	// fillExpires is the expiry of fill in Unix milliseconds.
	fillExpires int64
}
