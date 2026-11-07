package block_store_s3

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// windowFillBytes is the window around the block asked for whose complete
// blocks a filling read fetches with it. The transport's least window, 1
// MiB, already holds it unless the store reads exact extents.
const windowFillBytes = 1 << 20

// SetWindowFill writes every complete block a pack read fetches to target,
// verified and in one batch per fetch: the block asked for and its neighbours
// in the fetched window. Packs are cut in read order, so the neighbours are
// the blocks the next reads want. ctx scopes the writes, and a nil target
// stops them. Each pack reader hands a block to target at most once, unless
// the batch fails. An extent read, see SetIndexExtentReads, fetches one block
// and fills nothing.
func (s *PackStore) SetWindowFill(ctx context.Context, target block.StoreOps) {
	s.packs.SetWriteback(ctx, target, windowFillBytes)
}
