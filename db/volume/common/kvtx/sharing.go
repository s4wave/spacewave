package kvtx

import (
	"context"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// txHolders answers the sharing walk's questions from the blocks of one
// physical transaction.
type txHolders struct {
	// blocks serves the transaction's blocks.
	blocks block.StoreOps
}

// Stored reports whether the transaction holds the block node.
func (h txHolders) Stored(ctx context.Context, node string) (bool, error) {
	ref, ok := block_gc.ParseBlockIRI(node)
	if !ok {
		return false, nil
	}
	return h.blocks.GetBlockExists(ctx, ref)
}

// Implied returns the old root of the deadlist record a named root holds,
// pending or due: the record names its pages by value, so the graph has no
// edge from it to the root it replaced, which it holds until applied.
func (h txHolders) Implied(ctx context.Context, owner, object string) (string, error) {
	// Only a deadlist record's name implies a holder.
	_, name, ok := parseRootOwnerIRI(owner)
	deadlist := strings.HasPrefix(name, block.DeadlistRootPrefix) || strings.HasPrefix(name, block.DeadlistDueRootPrefix)
	if !ok || !deadlist {
		return "", nil
	}
	ref, ok := block_gc.ParseBlockIRI(object)
	if !ok {
		return "", nil
	}

	// Read the record's old root.
	first, _, err := block.ReadDeadlist(ctx, h.blocks, ref)
	if err != nil || first.GetFrom().GetEmpty() {
		return "", err
	}
	return block_gc.BlockIRI(first.GetFrom()), nil
}

// walkSharing runs the graph's sharing walk to its end in budgeted
// transactions, as RefGraph.WalkSharing describes, each yielding the writer
// after its budget or to a waiting writer. A cancelled walk keeps its place
// for the next block pass.
func (v *Volume) walkSharing(ctx context.Context) error {
	if !v.holdCounts {
		return nil
	}
	for {
		// Walk one slice.
		var s block_gc.SharingState
		err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) (err error) {
			held := time.Now()
			yield := func() bool { return time.Since(held) >= sweepBudget || v.writersWaiting() }
			s, err = rg.WalkSharing(ctx, yield)
			return err
		})
		if err != nil {
			return err
		}
		if s.Phase.Running() {
			continue
		}

		// Report a walk in doubt, whose counts may be short.
		if s.Doubt && v.le != nil {
			v.le.Warn("shared counts are in doubt: the sharing walk met a holder it cannot count")
		}
		return nil
	}
}
