package block

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/hash"
)

type pendingBlock struct {
	ref           *BlockRef
	data          []byte
	refs          []*BlockRef
	tombstone     bool
	queued        bool
	borrowed      bool
	metadataBytes int
}

type drainBatch struct {
	keys    []string
	entries []*PutBatchEntry
	pending []*pendingBlock
}

// BufferedStore buffers PutBlock calls in memory and drains them explicitly on
// Sync or when a caller must free capacity.
type BufferedStore struct {
	// inner is the store drains write to.
	inner StoreOps

	// bcast guards and signals the fields below.
	bcast broadcast.Broadcast
	// drainMu serializes drain batches against concurrent drainers.
	drainMu csync.Mutex
	// pending holds queued blocks keyed by ref id.
	pending map[string]*pendingBlock
	// pendingBytes is the total size of queued blocks.
	pendingBytes int
	// pendingMetadataBytes includes reference content and fixed entry overhead.
	pendingMetadataBytes    int
	maxPendingMetadataBytes int
	// maxPendingBytes caps pendingBytes before a forced drain.
	maxPendingBytes int
	// maxPendingBlocks caps len(pending) before a forced drain.
	maxPendingBlocks int
	// drainBatchEntries is the number of entries written per batch.
	drainBatchEntries int

	// queue preserves block arrival order across the pending map.
	queue []string

	// inFlight counts borrowed batches, which remain readable and capacity-accounted.
	inFlight int

	// drainErr captures the last drain error to surface on subsequent calls.
	drainErr error
}

// NewBufferedStore constructs a buffered store around an inner store.
func NewBufferedStore(ctx context.Context, inner StoreOps) *BufferedStore {
	return NewBufferedStoreWithSettings(ctx, inner, nil)
}

// NewBufferedStoreWithSettings constructs a buffered store with explicit settings.
func NewBufferedStoreWithSettings(
	_ context.Context,
	inner StoreOps,
	settings *BufferedStoreSettings,
) *BufferedStore {
	settings = normalizeBufferedStoreSettings(settings)
	return &BufferedStore{
		inner:                   inner,
		pending:                 make(map[string]*pendingBlock),
		maxPendingBytes:         settings.MaxPendingBytes,
		maxPendingMetadataBytes: settings.MaxPendingMetadataBytes,
		maxPendingBlocks:        settings.MaxPendingEntries,
		drainBatchEntries:       settings.DrainBatchEntries,
	}
}

// GetHashType returns the preferred hash type.
func (s *BufferedStore) GetHashType() hash.HashType {
	return s.inner.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *BufferedStore) GetSupportedFeatures() StoreFeature {
	return s.inner.GetSupportedFeatures()
}

// BeginReadOperation returns the buffered store for a read scope.
func (s *BufferedStore) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock buffers a block in memory and drains synchronously only when
// backpressure requires capacity.
func (s *BufferedStore) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return s.putBlock(ctx, data, opts, true)
}

// putBlock verifies and buffers a block, optionally resolving prior existence.
// Batch callers discard the existence result and let storage deduplicate at drain.
func (s *BufferedStore) putBlock(ctx context.Context, data []byte, opts *PutOpts, checkExists bool) (*BlockRef, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, ErrEmptyBlock
	}

	if opts == nil {
		opts = &PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	syncRequested := opts.GetSync()
	opts.Sync = false
	opts.HashType = opts.SelectHashType(s.inner.GetHashType())
	finish := func(ref *BlockRef, existed bool) (*BlockRef, bool, error) {
		if syncRequested {
			if _, err := s.Sync(ctx); err != nil {
				return ref, existed, err
			}
		}
		return ref, existed, nil
	}

	ref, err := BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forceRef := opts.GetForceBlockRef(); !forceRef.GetEmpty() {
		if !ref.EqualsRef(forceRef) {
			return ref, false, ErrBlockRefMismatch
		}
	}

	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, false, err
	}

	var drainErr error
	var existingPending *pendingBlock
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		drainErr = s.drainErr
		existingPending = s.pending[key]
	})
	if drainErr != nil {
		return nil, false, drainErr
	}
	if existingPending != nil && !existingPending.tombstone && containsBlockRefs(existingPending.refs, opts.GetRefs()) {
		return finish(ref, true)
	}

	var exists bool
	// A pending tombstone means this put must restore the block, even if it drains now.
	if checkExists && existingPending == nil {
		exists, err = s.inner.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, false, err
		}
	}

	// A single entry larger than the buffer cannot make progress by draining an
	// empty queue. Persist it through the existing durable preparation path.
	// This deliberately reports extra I/O rather than exceeding the memory cap.
	metadataBytes := bufferedMetadataSize(key, ref, opts.GetRefs(), s.maxPendingMetadataBytes)
	if (s.maxPendingBytes > 0 && len(data) > s.maxPendingBytes) ||
		metadataBytes > s.maxPendingMetadataBytes {
		if err := s.drainAll(ctx); err != nil {
			return nil, false, err
		}
		ref, existed, err := s.inner.PutBlock(ctx, data, opts)
		if err != nil {
			return nil, existed, err
		}
		return finish(ref, existed)
	}

	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/enqueue")
	defer subtask.End()
	pendingClone := &pendingBlock{
		ref:           ref.Clone(),
		data:          bytes.Clone(data),
		refs:          CloneBlockRefs(opts.GetRefs()),
		metadataBytes: metadataBytes,
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var done bool
		var alreadyExists bool
		var putErr error
		s.bcast.HoldLock(func(broadcastFn func(), getWaitCh func() <-chan struct{}) {
			if s.drainErr != nil {
				putErr = s.drainErr
				done = true
				return
			}
			if p := s.pending[key]; p != nil && !p.tombstone {
				alreadyExists = true
				if containsBlockRefs(p.refs, pendingClone.refs) {
					done = true
					return
				}
				// An opaque copy may acquire a decoder later. Preserve both
				// sets of dependencies without mutating a borrowed entry.
				merged := *pendingClone
				merged.refs = CloneBlockRefs(p.refs)
				for _, child := range pendingClone.refs {
					if !containsBlockRefs(merged.refs, []*BlockRef{child}) {
						merged.refs = append(merged.refs, child.Clone())
					}
				}
				merged.metadataBytes = bufferedMetadataSize(key, ref, merged.refs, s.maxPendingMetadataBytes)
				putErr = s.putPendingLocked(broadcastFn, key, &merged)
			} else {
				// Presence does not establish ownership in the destination.
				// Forward the write even when the physical bytes already exist.
				alreadyExists = exists
				putErr = s.putPendingLocked(broadcastFn, key, pendingClone)
			}
			if putErr == nil {
				done = true
				return
			}
			if putErr != ErrBufferedStoreFull {
				done = true
				return
			}
		})
		if done {
			if putErr != nil {
				return nil, false, putErr
			}
			if alreadyExists {
				return finish(ref, true)
			}
			return finish(ref, false)
		}
		_, drainTask := trace.NewTask(ctx, "hydra/block/buffered-store/enqueue/drain-capacity")
		if err := s.drainForCapacity(ctx); err != nil {
			drainTask.End()
			return nil, false, err
		}
		drainTask.End()
	}
}

// containsBlockRefs checks dependency coverage independently of ordering.
func containsBlockRefs(have, want []*BlockRef) bool {
	for _, ref := range want {
		if ref.GetEmpty() {
			continue
		}
		if !slices.ContainsFunc(have, ref.EqualsRef) {
			return false
		}
	}
	return true
}

// PutBlockBatch buffers verified entries without per-block existence probes.
// Pending entries deduplicate locally; durable duplicates resolve during drain.
func (s *BufferedStore) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			if err := s.RmBlock(ctx, entry.Ref); err != nil {
				return err
			}
			continue
		}
		var ref *BlockRef
		if entry.Ref != nil {
			ref = entry.Ref.Clone()
		}
		if _, _, err := s.putBlock(ctx, entry.Data, &PutOpts{
			ForceBlockRef: ref,
			Refs:          CloneBlockRefs(entry.Refs),
		}, false); err != nil {
			return err
		}
	}
	return nil
}

// GetBlock gets a block by reference.
func (s *BufferedStore) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	pending, err := s.getPending(ref)
	if err != nil {
		return nil, false, err
	}
	if pending != nil {
		if pending.tombstone {
			return nil, false, nil
		}
		return bytes.Clone(pending.data), true, nil
	}
	return s.inner.GetBlock(ctx, ref)
}

// GetStoredBlock gets a block and its references, preferring pending writes.
func (s *BufferedStore) GetStoredBlock(ctx context.Context, ref *BlockRef) (*StoredBlock, error) {
	pending, err := s.getPending(ref)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		if pending.tombstone {
			return nil, nil
		}
		return &StoredBlock{
			Data:      bytes.Clone(pending.data),
			Refs:      CloneBlockRefs(pending.refs),
			RefsKnown: true,
		}, nil
	}
	return s.inner.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks if a block exists.
func (s *BufferedStore) GetBlockExists(ctx context.Context, ref *BlockRef) (bool, error) {
	pending, err := s.getPending(ref)
	if err != nil {
		return false, err
	}
	if pending != nil {
		return !pending.tombstone, nil
	}
	return s.inner.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks if blocks exist.
func (s *BufferedStore) GetBlockExistsBatch(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	var missing []*BlockRef
	var missingIdx []int
	for i, ref := range refs {
		pending, err := s.getPending(ref)
		if err != nil {
			return nil, err
		}
		if pending != nil {
			out[i] = !pending.tombstone
			continue
		}
		missing = append(missing, ref)
		missingIdx = append(missingIdx, i)
	}
	if len(missing) == 0 {
		return out, nil
	}
	found, err := s.inner.GetBlockExistsBatch(ctx, missing)
	if err != nil {
		return nil, err
	}
	for i, ok := range found {
		out[missingIdx[i]] = ok
	}
	return out, nil
}

// RmBlock deletes a block by reference.
func (s *BufferedStore) RmBlock(ctx context.Context, ref *BlockRef) error {
	key, err := marshalRefKey(ref)
	if err != nil {
		return err
	}
	metadataBytes := bufferedMetadataSize(key, ref, nil, s.maxPendingMetadataBytes)
	if metadataBytes > s.maxPendingMetadataBytes {
		if err := s.drainAll(ctx); err != nil {
			return err
		}
		return s.inner.RmBlock(ctx, ref)
	}
	pendingClone := &pendingBlock{
		ref:           ref.Clone(),
		tombstone:     true,
		metadataBytes: metadataBytes,
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var done bool
		var rmErr error
		s.bcast.HoldLock(func(broadcastFn func(), getWaitCh func() <-chan struct{}) {
			if s.drainErr != nil {
				rmErr = s.drainErr
				done = true
				return
			}
			if p := s.pending[key]; p != nil && p.tombstone {
				done = true
				return
			}
			err := s.putPendingLocked(broadcastFn, key, pendingClone)
			if err == nil {
				done = true
				return
			}
			if err != ErrBufferedStoreFull {
				rmErr = err
				done = true
				return
			}
		})
		if done {
			return rmErr
		}
		if err := s.drainForCapacity(ctx); err != nil {
			return err
		}
	}
}

// StatBlock returns metadata about a block without reading its data.
func (s *BufferedStore) StatBlock(ctx context.Context, ref *BlockRef) (*BlockStat, error) {
	pending, err := s.getPending(ref)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		if pending.tombstone {
			return nil, nil
		}
		return &BlockStat{
			Ref:  pending.ref.Clone(),
			Size: int64(len(pending.data)),
		}, nil
	}
	return s.inner.StatBlock(ctx, ref)
}

// Sync drains buffered blocks, then forwards the durability barrier to inner.
// Draining is owned solely by Sync (and by backpressure inside PutBlock).
func (s *BufferedStore) Sync(ctx context.Context) (bool, error) {
	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/sync/wait-durable")
	defer subtask.End()
	if err := s.drainAll(ctx); err != nil {
		return false, err
	}
	return s.inner.Sync(ctx)
}

// BeginDeferFlush forwards the GC defer-flush scope to the inner store.
func (s *BufferedStore) BeginDeferFlush() {
	BeginDeferFlush(s.inner)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the inner store.
// Buffered blocks are never drained here; only Sync drains.
func (s *BufferedStore) EndDeferFlush(ctx context.Context) error {
	return EndDeferFlush(ctx, s.inner)
}

// drainForCapacity drains batches until the pending queue is within its
// configured limits.
func (s *BufferedStore) drainForCapacity(ctx context.Context) error {
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	defer release()

	// Another writer may have drained while we waited for drainMu. A missing
	// batch is not terminal; the caller retries the enqueue path.
	_, err = s.drainNextBatch(ctx)
	return err
}

// drainAll drains every queued block.
func (s *BufferedStore) drainAll(ctx context.Context) error {
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	defer release()

	for {
		drained, err := s.drainNextBatch(ctx)
		if err != nil {
			return err
		}
		if !drained {
			return nil
		}
	}
}

// logPendingShape logs the current queue depth and byte count under the
// given trace category.
func (s *BufferedStore) logPendingShape(ctx context.Context, category string) {
	var pending int
	var queued int
	var pendingBytes int
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pending = len(s.pending)
		queued = len(s.queue)
		pendingBytes = s.pendingBytes
	})
	trace.Logf(ctx, category, "pending=%d queued=%d bytes=%d", pending, queued, pendingBytes)
}

// drainNextBatch writes one batch of queued blocks, returning false when
// the queue is empty or the batch was returned for retry.
func (s *BufferedStore) drainNextBatch(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var batch *drainBatch
	var drainErr error
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if drainErr = ctx.Err(); drainErr != nil {
			return
		}
		if s.drainErr != nil {
			drainErr = s.drainErr
			return
		}
		var subtask *trace.Task
		_, subtask = trace.NewTask(ctx, "hydra/block/buffered-store/drain/take-batch")
		batch = s.takeDrainBatchLocked(s.drainBatchEntries)
		subtask.End()
	})
	if drainErr != nil {
		return false, drainErr
	}
	if batch == nil {
		var wait <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			if s.inFlight != 0 {
				wait = getWait()
			}
		})
		if wait != nil {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-wait:
			}
			return true, nil // Retry after the borrowed publication settles.
		}
		return false, nil
	}

	writeCtx, writeTask := trace.NewTask(ctx, "hydra/block/buffered-store/drain/write-batch")
	err := s.writeBatch(writeCtx, batch.entries)
	writeTask.End()

	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		s.completeBatchLocked(batch, err)
		if err != nil && ctx.Err() == nil {
			s.drainErr = err
		}
		broadcastFn()
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return true, ctxErr
		}
		return true, err
	}
	return true, nil
}

// takeDrainBatchLocked pops the next batch from the queue, bounded by
// maxEntries when positive. Caller must hold bcast lock.
func (s *BufferedStore) takeDrainBatchLocked(maxEntries int) *drainBatch {
	if len(s.queue) == 0 {
		return nil
	}

	keys := s.queue
	if maxEntries > 0 && len(keys) > maxEntries {
		keys = slices.Clone(keys[:maxEntries])
		s.queue = s.queue[maxEntries:]
	} else {
		keys = slices.Clone(keys)
		s.queue = nil
	}

	batch := &drainBatch{
		keys:    keys,
		entries: make([]*PutBatchEntry, 0, len(keys)),
	}
	for _, key := range keys {
		pending := s.pending[key]
		if pending == nil {
			continue
		}
		if !pending.queued || pending.borrowed {
			continue
		}
		pending.queued = false
		pending.borrowed = true
		batch.pending = append(batch.pending, pending)
		// Pending content is immutable after enqueue; replacements allocate a
		// new pendingBlock. The drain borrows the same content as its data bytes.
		batch.entries = append(batch.entries, &PutBatchEntry{
			Ref:       pending.ref,
			Data:      pending.data,
			Refs:      pending.refs,
			Tombstone: pending.tombstone,
		})
	}
	if len(batch.entries) == 0 {
		return nil
	}
	s.inFlight++
	return batch
}

// completeBatchLocked releases immutable borrowed content after its writer has
// stopped using it. Failure restores it for retry without poisoning the buffer:
// a rejected head CAS is not a block-storage failure.
func (s *BufferedStore) completeBatchLocked(batch *drainBatch, err error) {
	s.inFlight--
	var retry []string
	for _, p := range batch.pending {
		key, _ := marshalRefKey(p.ref)
		p.borrowed = false
		if s.pending[key] != p {
			continue
		}
		if err != nil {
			p.queued = true
			retry = append(retry, key)
		} else {
			s.pendingBytes -= len(p.data)
			s.pendingMetadataBytes -= p.metadataBytes
			delete(s.pending, key)
		}
	}
	s.queue = append(retry, s.queue...)
}

// PendingBatch is an immutable borrow of buffered entries. Complete must be
// called exactly once after the publisher is finished, even on admission failure.
// Entries remain readable through the BufferedStore until successful completion.
type PendingBatch struct {
	// Entries are the borrowed batch entries.
	Entries []*PutBatchEntry
	// complete resolves the borrow with the publication result.
	complete func(error)
	// completed records that Complete already ran; further calls are no-ops.
	completed bool
}

// Complete resolves the borrow exactly once with the publication result.
func (b *PendingBatch) Complete(err error) { b.complete(err) }

// TakePending borrows all currently queued entries without writing or forgetting
// them. Further writes can prepare the next publication while this borrow is in
// flight. Both queued and borrowed content count against the existing bounds.
func (s *BufferedStore) TakePending(ctx context.Context) (*PendingBatch, error) {
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var batch *drainBatch
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if err = ctx.Err(); err != nil {
			return
		}
		if err = s.drainErr; err != nil {
			return
		}
		batch = s.takeDrainBatchLocked(0)
	})
	if err != nil {
		return nil, err
	}
	out := &PendingBatch{complete: func(error) {}}
	if batch != nil {
		out.Entries = batch.entries
		out.complete = func(err error) {
			s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
				if out.completed {
					return
				}
				out.completed = true
				s.completeBatchLocked(batch, err)
				broadcastFn()
			})
		}
	}
	return out, nil
}

// writeBatch writes the entries to the inner store as one put batch.
func (s *BufferedStore) writeBatch(ctx context.Context, entries []*PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	batchCtx, batchTask := trace.NewTask(ctx, "hydra/block/buffered-store/write-batch/put-block-batch")
	err := s.inner.PutBlockBatch(batchCtx, entries)
	batchTask.End()
	return err
}

// marshalRefKey marshals the ref into its queue key.
func marshalRefKey(ref *BlockRef) (string, error) {
	dat, err := ref.MarshalKey()
	if err != nil {
		return "", err
	}
	return string(dat), nil
}

// getPending returns the queued block for the ref, or nil.
func (s *BufferedStore) getPending(ref *BlockRef) (*pendingBlock, error) {
	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, err
	}
	var pending *pendingBlock
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pending = s.pending[key]
	})
	return pending, nil
}

// putPendingLocked queues or replaces a pending block. Caller must hold
// bcast lock.
func (s *BufferedStore) putPendingLocked(broadcastFn func(), key string, pending *pendingBlock) error {
	prev := s.pending[key]
	// Keep the last submitted operation on this key ordered before a replacement.
	if prev != nil && prev.borrowed {
		return ErrBufferedStoreFull
	}
	prevBytes := 0
	prevMetadataBytes := 0
	if prev != nil {
		prevBytes = len(prev.data)
		prevMetadataBytes = prev.metadataBytes
	}
	pendingBytes := 0
	if pending != nil {
		pendingBytes = len(pending.data)
	}
	nextBytes := s.pendingBytes - prevBytes + pendingBytes
	nextMetadataBytes := s.pendingMetadataBytes - prevMetadataBytes + pending.metadataBytes
	if nextMetadataBytes > s.maxPendingMetadataBytes {
		return ErrBufferedStoreFull
	}
	if prev == nil && s.maxPendingBlocks > 0 && len(s.pending) >= s.maxPendingBlocks {
		return ErrBufferedStoreFull
	}
	if s.maxPendingBytes > 0 && nextBytes > s.maxPendingBytes {
		return ErrBufferedStoreFull
	}

	enqueue := prev == nil || !prev.queued
	pending.queued = true
	s.pending[key] = pending
	s.pendingBytes = nextBytes
	s.pendingMetadataBytes = nextMetadataBytes
	if enqueue {
		s.queue = append(s.queue, key)
		broadcastFn()
	}
	return nil
}

// bufferedMetadataSize charges reference bytes and conservative fixed object,
// slice/map, and queue overhead. This is an admission accounting bound, not a
// claim about allocator RSS. Stop once oversized rather than overflow the sum.
func bufferedMetadataSize(key string, ref *BlockRef, refs []*BlockRef, limit int) int {
	size := len(key) + ref.SizeVT() + 256
	if size > limit {
		return limit + 1
	}
	for _, r := range refs {
		charge := r.SizeVT() + 128
		if charge > limit-size {
			return limit + 1
		}
		size += charge
	}
	return size
}

// _ is a type assertion.
var (
	_ StoreOps = (*BufferedStore)(nil)
)
