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

	// pins holds the reader pins of pending roots by ref key until the drain
	// writes each root.
	pins map[string][]*bufferedPin

	// written maps the ref key of each block written to inner to its outgoing
	// refs. Nil unless the store records writes.
	written map[string]*writtenBlock

	// withoutPeerWait makes inner reads report ErrUnavailable instead of
	// waiting for a peer to connect.
	withoutPeerWait bool
}

// writtenBlock is a block the store wrote to its inner store.
type writtenBlock struct {
	ref  *BlockRef
	refs []*BlockRef
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
	s := &BufferedStore{
		inner:                   inner,
		pending:                 make(map[string]*pendingBlock),
		pins:                    make(map[string][]*bufferedPin),
		maxPendingBytes:         settings.MaxPendingBytes,
		maxPendingMetadataBytes: settings.MaxPendingMetadataBytes,
		maxPendingBlocks:        settings.MaxPendingEntries,
		drainBatchEntries:       settings.DrainBatchEntries,
		withoutPeerWait:         settings.WithoutPeerWait,
	}
	if settings.RecordWrites {
		s.written = make(map[string]*writtenBlock)
	}
	return s
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
	// Require a live write context and nonempty block data.
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, ErrEmptyBlock
	}

	// Prepare private write options with the destination hash type.
	if opts == nil {
		opts = &PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	syncRequested := opts.GetSync()
	opts.Sync = false
	opts.HashType = opts.SelectHashType(s.inner.GetHashType())

	// Complete buffered writes with the requested durability barrier.
	finish := func(ref *BlockRef, existed bool) (*BlockRef, bool, error) {
		if syncRequested {
			if _, err := s.Sync(ctx); err != nil {
				return ref, existed, err
			}
		}
		return ref, existed, nil
	}

	// Verify the block content against its requested reference.
	ref, err := BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forceRef := opts.GetForceBlockRef(); !forceRef.GetEmpty() {
		if !ref.EqualsRef(forceRef) {
			return ref, false, ErrBlockRefMismatch
		}
	}

	// Encode the block reference for the pending queue.
	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, false, err
	}

	// Reuse pending content that already covers the requested dependencies.
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

	// Track prior block existence for the buffered write result.
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
		// Drain queued blocks before writing an oversized entry directly.
		if err := s.drainAll(ctx); err != nil {
			return nil, false, err
		}

		// Persist the oversized block through the inner store.
		ref, existed, err := s.inner.PutBlock(ctx, data, opts)
		if err != nil {
			return nil, existed, err
		}

		// Record the direct write in the buffered store history.
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			s.recordWriteLocked(key, ref, opts.GetRefs(), false)
		})
		return finish(ref, existed)
	}

	// Retain immutable block content while acquiring buffer capacity.
	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/enqueue")
	defer subtask.End()
	pendingClone := &pendingBlock{
		ref:           ref.Clone(),
		data:          bytes.Clone(data),
		refs:          CloneBlockRefs(opts.GetRefs()),
		metadataBytes: metadataBytes,
	}
	for {
		// Require a live write context before acquiring queue capacity.
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}

		// Enqueue the block under the buffered store state lock.
		var done bool
		var alreadyExists bool
		var putErr error
		s.bcast.HoldLock(func(broadcastFn func(), getWaitCh func() <-chan struct{}) {
			// Surface a previous drain failure before changing pending content.
			if s.drainErr != nil {
				putErr = s.drainErr
				done = true
				return
			}

			// Merge dependencies into an existing block or enqueue a new block.
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

			// Complete the enqueue operation unless draining can free capacity.
			if putErr == nil {
				done = true
				return
			}
			if putErr != ErrBufferedStoreFull {
				done = true
				return
			}
		})

		// Return the completed buffered write or its failure.
		if done {
			if putErr != nil {
				return nil, false, putErr
			}
			if alreadyExists {
				return finish(ref, true)
			}
			return finish(ref, false)
		}

		// Drain pending blocks to make room for the requested write.
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
	return s.inner.GetBlock(s.innerReadContext(ctx), ref)
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
	return s.inner.GetStoredBlock(s.innerReadContext(ctx), ref)
}

// innerReadContext returns the context for a read of the inner store.
func (s *BufferedStore) innerReadContext(ctx context.Context) context.Context {
	if s.withoutPeerWait {
		ctx, _ = WithoutPeerWait(ctx)
	}
	return ctx
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
	// Resolve pending block existence and collect refs needing storage lookup.
	out := make([]bool, len(refs))
	var missing []*BlockRef
	var missingIdx []int
	for i, ref := range refs {
		// Read pending existence before consulting the inner block store.
		pending, err := s.getPending(ref)
		if err != nil {
			return nil, err
		}

		// Use buffered tombstone state for refs already queued.
		if pending != nil {
			out[i] = !pending.tombstone
			continue
		}

		// Retain unresolved refs and their result positions.
		missing = append(missing, ref)
		missingIdx = append(missingIdx, i)
	}

	// Return the buffered results when no storage lookup remains.
	if len(missing) == 0 {
		return out, nil
	}

	// Query the inner store for refs absent from the pending queue.
	found, err := s.inner.GetBlockExistsBatch(ctx, missing)
	if err != nil {
		return nil, err
	}

	// Merge stored block existence into the original ref order.
	for i, ok := range found {
		out[missingIdx[i]] = ok
	}
	return out, nil
}

// RmBlock deletes a block by reference.
func (s *BufferedStore) RmBlock(ctx context.Context, ref *BlockRef) error {
	// Encode the reference for the buffered deletion.
	key, err := marshalRefKey(ref)
	if err != nil {
		return err
	}

	// Delete oversized references directly after draining pending content.
	metadataBytes := bufferedMetadataSize(key, ref, nil, s.maxPendingMetadataBytes)
	if metadataBytes > s.maxPendingMetadataBytes {
		if err := s.drainAll(ctx); err != nil {
			return err
		}
		return s.inner.RmBlock(ctx, ref)
	}

	// Retain a tombstone while acquiring capacity for the deletion.
	pendingClone := &pendingBlock{
		ref:           ref.Clone(),
		tombstone:     true,
		metadataBytes: metadataBytes,
	}
	for {
		// Require a live deletion context before acquiring queue capacity.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Enqueue the tombstone under the buffered store state lock.
		var done bool
		var rmErr error
		s.bcast.HoldLock(func(broadcastFn func(), getWaitCh func() <-chan struct{}) {
			// Surface a previous drain failure before changing pending content.
			if s.drainErr != nil {
				rmErr = s.drainErr
				done = true
				return
			}

			// Reuse an existing pending tombstone for the block.
			if p := s.pending[key]; p != nil && p.tombstone {
				done = true
				return
			}

			// Queue the tombstone and distinguish capacity exhaustion from failure.
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

		// Return the completed deletion or its failure.
		if done {
			return rmErr
		}

		// Drain pending blocks to make room for the tombstone.
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
// Draining is owned by Sync, Flush, and backpressure inside PutBlock.
func (s *BufferedStore) Sync(ctx context.Context) (bool, error) {
	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/sync/wait-durable")
	defer subtask.End()
	if err := s.drainAll(ctx); err != nil {
		return false, err
	}
	return s.inner.Sync(ctx)
}

// Flush drains buffered blocks through every buffered layer into the first
// unbuffered store, without its durability barrier.
func (s *BufferedStore) Flush(ctx context.Context) error {
	// Drain this buffered layer while tracing its flush operation.
	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/flush")
	defer subtask.End()
	if err := s.drainAll(ctx); err != nil {
		return err
	}

	// Flush an inner buffered layer after draining this layer.
	if inner, ok := s.inner.(*BufferedStore); ok {
		return inner.Flush(ctx)
	}
	return nil
}

// BeginDeferFlush forwards the GC defer-flush scope to the inner store.
func (s *BufferedStore) BeginDeferFlush() {
	BeginDeferFlush(s.inner)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the inner store.
// Buffered blocks are never drained here; only Sync and Flush drain.
func (s *BufferedStore) EndDeferFlush(ctx context.Context) error {
	return EndDeferFlush(ctx, s.inner)
}

// drainForCapacity drains batches until the pending queue is within its
// configured limits.
func (s *BufferedStore) drainForCapacity(ctx context.Context) error {
	// Serialize capacity drains with other buffered writers.
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
	return s.drainQueue(ctx)
}

// drainQueue writes batches until the queue is empty. Caller holds drainMu.
func (s *BufferedStore) drainQueue(ctx context.Context) error {
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
	// Capture the pending queue dimensions under the store state lock.
	var pending int
	var queued int
	var pendingBytes int
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pending = len(s.pending)
		queued = len(s.queue)
		pendingBytes = s.pendingBytes
	})

	// Log the captured queue dimensions in the requested trace category.
	trace.Logf(ctx, category, "pending=%d queued=%d bytes=%d", pending, queued, pendingBytes)
}

// drainNextBatch writes one batch of queued blocks, returning false when
// the queue is empty or the batch was returned for retry.
func (s *BufferedStore) drainNextBatch(ctx context.Context) (bool, error) {
	// Require a live drain context before borrowing pending blocks.
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Borrow the next queued batch under the store state lock.
	var batch *drainBatch
	var drainErr error
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Surface cancellation or a previous drain failure before borrowing.
		if drainErr = ctx.Err(); drainErr != nil {
			return
		}
		if s.drainErr != nil {
			drainErr = s.drainErr
			return
		}

		// Trace the bounded batch borrow from the pending queue.
		var subtask *trace.Task
		_, subtask = trace.NewTask(ctx, "hydra/block/buffered-store/drain/take-batch")
		batch = s.takeDrainBatchLocked(s.drainBatchEntries)
		subtask.End()
	})

	// Return a failed borrow before writing pending blocks.
	if drainErr != nil {
		return false, drainErr
	}

	// Wait for borrowed content to settle when no queued batch remains.
	if batch == nil {
		// Capture the notification for an outstanding publication.
		var wait <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			if s.inFlight != 0 {
				wait = getWait()
			}
		})

		// Wait for publication completion or drain cancellation.
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

	// Write the borrowed block batch to the inner store.
	writeCtx, writeTask := trace.NewTask(ctx, "hydra/block/buffered-store/drain/write-batch")
	err := s.writeBatch(writeCtx, batch.entries)
	writeTask.End()

	// Complete the batch and publish any persistent drain failure.
	var pins []*bufferedPin
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		s.completeBatchLocked(batch, err)
		if err == nil {
			pins = s.takeWrittenPinsLocked(batch)
		} else if ctx.Err() == nil {
			s.drainErr = err
		}
		broadcastFn()
	})

	// Move the reader pins of written roots to the inner store. A pin that
	// fails to hold poisons the buffer, since its holder relies on it.
	if err == nil {
		if err = s.acquirePins(ctx, pins); err != nil {
			s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
				s.drainErr = err
				broadcastFn()
			})
		}
	}
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
	// Stop borrowing batches when the pending queue is empty.
	if len(s.queue) == 0 {
		return nil
	}

	// Remove a bounded set of keys from the pending queue.
	keys := s.queue
	if maxEntries > 0 && len(keys) > maxEntries {
		keys = slices.Clone(keys[:maxEntries])
		s.queue = s.queue[maxEntries:]
	} else {
		keys = slices.Clone(keys)
		s.queue = nil
	}

	// Borrow immutable pending content for the selected queue keys.
	batch := &drainBatch{
		keys:    keys,
		entries: make([]*PutBatchEntry, 0, len(keys)),
	}
	for _, key := range keys {
		// Select queued blocks that are available for borrowing.
		pending := s.pending[key]
		if pending == nil {
			continue
		}
		if !pending.queued || pending.borrowed {
			continue
		}

		// Retain each selected block as borrowed batch content.
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

	// Discard empty batches before accounting for an active borrow.
	if len(batch.entries) == 0 {
		return nil
	}

	// Account for the borrowed batch until its publication completes.
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
		if err == nil {
			s.recordWriteLocked(key, p.ref, p.refs, p.tombstone)
		}
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

// writeBatch writes the entries to the inner store as one put batch.
func (s *BufferedStore) writeBatch(ctx context.Context, entries []*PutBatchEntry) error {
	// Skip storage writes for an empty block batch.
	if len(entries) == 0 {
		return nil
	}

	// Write the block batch while tracing the inner store operation.
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
	// Encode the reference for a pending block lookup.
	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, err
	}

	// Read pending content under the buffered store state lock.
	var pending *pendingBlock
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pending = s.pending[key]
	})
	return pending, nil
}

// putPendingLocked queues or replaces a pending block. Caller must hold
// bcast lock.
func (s *BufferedStore) putPendingLocked(broadcastFn func(), key string, pending *pendingBlock) error {
	// Find the pending entry being replaced by this write.
	prev := s.pending[key]

	// Keep the last submitted operation on this key ordered before a replacement.
	if prev != nil && prev.borrowed {
		return ErrBufferedStoreFull
	}

	// Account for the previous entry content being replaced.
	prevBytes := 0
	prevMetadataBytes := 0
	if prev != nil {
		prevBytes = len(prev.data)
		prevMetadataBytes = prev.metadataBytes
	}

	// Account for the new pending block content.
	pendingBytes := 0
	if pending != nil {
		pendingBytes = len(pending.data)
	}

	// Require the replacement to fit the buffered store capacity limits.
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

	// Publish the replacement entry and notify waiting drainers.
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
