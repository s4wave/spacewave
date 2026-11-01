package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/net/hash"
)

// BlockMark names one block a write stores.
type BlockMark struct {
	// Hash is the block hash.
	Hash *hash.Hash
	// Size is the block size in bytes.
	Size int64
}

// BlockMarker records the blocks a write stores in the transaction that stores
// them, so the blocks and their records commit or discard together.
type BlockMarker interface {
	// MarkerStoreID returns the ID of the object store of the volume that
	// holds the records. The writing transaction opens it.
	MarkerStoreID() string
	// MarkBlocks writes the records of blocks into tx, a view of the marker
	// store inside the transaction that stores the blocks.
	//
	// It must not commit or discard tx. The transaction may commit a block it
	// marked before, so MarkBlocks must accept a block that already has a
	// record.
	//
	// Returns a function to call once the transaction committed, nil for
	// none. A discarded transaction never calls it.
	MarkBlocks(ctx context.Context, tx Tx, blocks []BlockMark) (func(), error)
}
