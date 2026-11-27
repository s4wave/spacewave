package block

import (
	"context"
)

// WalkGraph visits every block under roots once by following the outgoing
// refs src reports, keeping reads blocks in flight in a few batch reads, as
// graphReader describes. Zero reads selects DefaultGraphCopyReads. It never
// decodes a block.
//
// Returns ErrNotFound for a missing block and ErrRefsUnknown for a block src
// holds without its refs, each wrapped with the block ref. visit runs on the
// calling goroutine, one block at a time. The callback returns ErrSkipRefs to
// leave the visited block's refs unwalked.
func WalkGraph(ctx context.Context, src StoreOps, roots []*BlockRef, reads int, visit func(ref *BlockRef, stored *StoredBlock) error) error {
	// Queue each distinct non-empty root.
	seen := make(map[string]struct{})
	var queue []*BlockRef
	push := func(ref *BlockRef) {
		key := ref.MapKey()
		if _, ok := seen[key]; ok || ref.GetEmpty() {
			return
		}
		seen[key] = struct{}{}
		queue = append(queue, ref)
	}
	for _, root := range roots {
		push(root)
	}

	// Read blocks depth first until the queue drains.
	ctx, cancel := context.WithCancel(ctx)
	reader := newGraphReader(src, reads, func(ref *BlockRef) *BlockRef { return ref })
	defer func() {
		cancel()
		reader.close()
	}()
	for {
		queue = reader.fill(ctx, queue)
		res, ok := reader.next()
		if !ok {
			return nil
		}
		if res.err != nil {
			return res.err
		}
		if res.end {
			continue
		}

		// Visit one block and queue its unseen children.
		if visit != nil {
			err := visit(res.node, res.stored)
			if err == ErrSkipRefs {
				continue
			}
			if err != nil {
				return err
			}
		}
		for _, ref := range res.stored.Refs {
			push(ref)
		}
	}
}
