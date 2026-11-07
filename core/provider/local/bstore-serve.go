package provider_local

import "sync/atomic"

// serveOnly is the serve mode of the block stores mounted after it is set.
var serveOnly atomic.Bool

// SetServeOnly puts the block stores mounted after the call in serve mode, for
// a backend that only serves its blocks to the World's new owner. A read in
// serve mode writes nothing: no fill of the local store or the fill cache, no
// saved reads, and no peer reads, since the peer asking is the new owner. The
// storage node's first answer is final, so a block every tier answered missing
// is not found at once, and a node that is away or silent fails the read with
// block.ErrUnavailable, which the new owner retries. The upload of queued
// writes and eviction run as before.
func SetServeOnly(on bool) {
	serveOnly.Store(on)
}
