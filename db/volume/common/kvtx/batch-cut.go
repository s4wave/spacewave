package kvtx

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// backgroundBatchHold is how long a background batch put holds the store's
// writer turn before it commits what it wrote and queues for the turn again.
// A foreground writer that arrives during a background batch waits at most
// about this long, plus one cut slice and the commit, instead of the whole
// batch. Zero puts each batch in one transaction.
var backgroundBatchHold = 50 * time.Millisecond

// batchCutEntries is how many entries a cut batch writes between its checks
// of the time held and of a waiting foreground writer. It bounds how far a
// transaction overruns backgroundBatchHold.
const batchCutEntries = 128

// putOwnedCut writes entries under owner, as PrepareOwnedBlockBatch does, in
// as many transactions as it takes to hold the writer turn at most about
// backgroundBatchHold each, committing early when a foreground writer waits.
//
// A batch cut into several transactions is equivalent to one. Each cut
// commits its blocks with their ownership by owner, claimed in the same
// transaction, so no collection between cuts finds a block of the batch
// unowned. Without an owner the bucket handle keeps no collection state, which
// it does only while the Volume's collection is off (GCDisabled), and then
// nothing deletes a block between cuts: a crash between cuts leaves stored
// blocks that nothing names yet, which a retried batch finds stored, as after
// any interrupted put. An ownerless cut batch is safe only while collection
// is off; with collection on the bucket handle names an owner.
func (v *Volume) putOwnedCut(ctx context.Context, owner string, entries []*block.PutBatchEntry) ([]bool, error) {
	existed := make([]bool, 0, len(entries))
	size := batchBytes(entries)
	for remaining := entries; len(remaining) != 0; {
		// Write slices of the remaining entries until the turn has been held
		// long enough or a foreground writer waits.
		var written int
		var part []bool
		err := v.prepareOwned(ctx, v.withDirectAtomic, owner, size, claimBucket, func(store block.StoreOps) error {
			held := time.Now()
			for written < len(remaining) {
				end := min(written+batchCutEntries, len(remaining))
				got, err := store.PutBlockBatch(ctx, remaining[written:end])
				if err != nil {
					return err
				}
				part = append(part, got...)
				written = end
				if time.Since(held) >= backgroundBatchHold || v.foregroundWaiting() {
					break
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		// Later cuts check the reserve state for the bytes still to write.
		size -= batchBytes(remaining[:written])
		existed = append(existed, part...)
		remaining = remaining[written:]
	}
	return existed, nil
}
