package block

import "context"

// recordWriteLocked records a block written to inner. A written tombstone
// already released the block, so it leaves the record. Caller holds bcast.
func (s *BufferedStore) recordWriteLocked(key string, ref *BlockRef, refs []*BlockRef, tombstone bool) {
	if s.written == nil {
		return
	}
	if tombstone {
		delete(s.written, key)
		return
	}
	s.written[key] = &writtenBlock{ref: ref.Clone(), refs: CloneBlockRefs(refs)}
}

// ReleaseUnreached drops the staging ownership the inner store holds for each
// recorded block that roots do not reach through recorded references, then
// clears the record. Blocks of an abandoned write and blocks a later write
// replaced then survive only through another owner, such as a parent block
// written before this store existed.
//
// Call it once the writer is done: blocks still pending are not recorded, and
// a block released here is collectable by the next sweep. It waits for a drain
// in progress so the record includes the blocks that drain writes.
func (s *BufferedStore) ReleaseUnreached(ctx context.Context, roots ...*BlockRef) error {
	// Take the record after any drain in progress.
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		return err
	}
	var written map[string]*writtenBlock
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		written = s.written
		if written != nil {
			s.written = make(map[string]*writtenBlock)
		}
	})
	release()
	if len(written) == 0 {
		return nil
	}

	// Remove every recorded block the roots reach.
	stack := append([]*BlockRef(nil), roots...)
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if ref.GetEmpty() {
			continue
		}
		key, err := marshalRefKey(ref)
		if err != nil {
			return err
		}
		blk, ok := written[key]
		if !ok {
			continue
		}
		delete(written, key)
		stack = append(stack, blk.refs...)
	}

	// Release the rest.
	unreached := make([]*BlockRef, 0, len(written))
	for _, blk := range written {
		unreached = append(unreached, blk.ref)
	}
	return ReleaseRoots(ctx, s.inner, unreached)
}
