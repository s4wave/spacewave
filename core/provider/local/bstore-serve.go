package provider_local

import (
	"sync/atomic"

	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
)

// serveOnly is the serve mode of the block stores mounted after it is set.
var serveOnly atomic.Bool

// SetServeOnly puts the block stores mounted after the call in serve mode, for
// a backend that only serves its blocks to the World's new owner. A read in
// serve mode writes nothing: no fill of the local store or the fill cache, no
// saved reads, and no peer reads, since the peer asking is the new owner. The
// storage node's first answer is final, so a block every tier answered missing
// is not found at once, and a node that is away or silent fails the read with
// block.ErrUnavailable, which the new owner retries.
//
// Serve mode starts no writer either: no upload or drain of queued writes, no
// eviction, no backfill, no push to the storage node and no apply of the
// packs it reports, and no SQL publisher. The storage node is read by key
// alone. The upload queue opens in a read transaction, as
// block_store_writeback.SetReadOnlyOpen describes, and the pack stores of
// FTPS, SFTP and S3 placements write nothing to their index, as
// PackStoreOptions.ReadOnlyIndex describes.
func SetServeOnly(on bool) {
	serveOnly.Store(on)
	block_store_writeback.SetReadOnlyOpen(on)
}

// peerFillsUpload is the new owner's peer fill mode of the block stores mounted
// after it is set.
var peerFillsUpload atomic.Bool

// SetPeerFillsUpload makes the block stores mounted after the call store a
// block read from a peer as a block only the read fallback holds: with an
// upload marker, so it uploads to the backend, and eviction keeps it until
// then. It is for the World's new owner, whose peer is the old owner and
// holds the blocks the backend lacks; a peer is read only after the backend
// missed. Off, a peer's block fills the local store as a backend hit does,
// with no marker.
func SetPeerFillsUpload(on bool) {
	peerFillsUpload.Store(on)
}
