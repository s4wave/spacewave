package block

import (
	"context"

	"github.com/pkg/errors"
)

// WalkGraph visits every block under roots once by following the outgoing
// refs src reports, keeping reads source reads in flight. Zero reads selects
// DefaultGraphCopyReads. It never decodes a block.
//
// Returns ErrNotFound for a missing block and ErrRefsUnknown for a block src
// holds without its refs, each wrapped with the block ref. visit runs on the
// calling goroutine, one block at a time.
func WalkGraph(ctx context.Context, src StoreOps, roots []*BlockRef, reads int, visit func(ref *BlockRef, stored *StoredBlock) error) error {
	// Queue each distinct non-empty root.
	seen := make(map[string]struct{})
	var queue []*BlockRef
	push := func(ref *BlockRef) {
		key := ref.MarshalString()
		if _, ok := seen[key]; ok || ref.GetEmpty() {
			return
		}
		seen[key] = struct{}{}
		queue = append(queue, ref)
	}
	for _, root := range roots {
		push(root)
	}

	// Start the bounded reader pool.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if reads <= 0 {
		reads = DefaultGraphCopyReads
	}
	results := make(chan graphWalkRead, reads)
	inFlight := 0
	defer func() {
		cancel()
		for ; inFlight != 0; inFlight-- {
			<-results
		}
	}()

	// Read blocks depth first until the queue drains.
	for {
		for inFlight < reads && len(queue) != 0 {
			ref := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			inFlight++
			go func() {
				results <- readWalkBlock(ctx, src, ref)
			}()
		}
		if inFlight == 0 {
			return nil
		}

		// Visit one block and queue its unseen children.
		res := <-results
		inFlight--
		if res.err != nil {
			return res.err
		}
		if visit != nil {
			if err := visit(res.ref, res.stored); err != nil {
				return err
			}
		}
		for _, ref := range res.stored.Refs {
			push(ref)
		}
	}
}

// graphWalkRead is the result of reading one block of a walk.
type graphWalkRead struct {
	ref    *BlockRef
	stored *StoredBlock
	err    error
}

// readWalkBlock reads one block with its refs from src.
func readWalkBlock(ctx context.Context, src StoreOps, ref *BlockRef) graphWalkRead {
	stored, err := src.GetStoredBlock(ctx, ref)
	if err == nil && stored == nil {
		err = ErrNotFound
	}
	if err == nil && !stored.RefsKnown {
		err = ErrRefsUnknown
	}
	return graphWalkRead{ref: ref, stored: stored, err: errors.Wrap(err, ref.MarshalString())}
}
