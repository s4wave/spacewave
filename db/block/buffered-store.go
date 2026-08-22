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
	ref       *BlockRef
	data      []byte
	refs      []*BlockRef
	tombstone bool
	queued    bool
}

type drainBatch struct {
	keys    []string
	entries []*PutBatchEntry
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
	// maxPendingBytes caps pendingBytes before a forced drain.
	maxPendingBytes int
	// maxPendingBlocks caps len(pending) before a forced drain.
	maxPendingBlocks int
	// drainBatchEntries is the number of entries written per batch.
	drainBatchEntries int

	// queue preserves block arrival order across the pending map.
	queue []string

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
		inner:             inner,
		pending:           make(map[string]*pendingBlock),
		maxPendingBytes:   settings.MaxPendingBytes,
		maxPendingBlocks:  settings.MaxPendingEntries,
		drainBatchEntries: settings.DrainBatchEntries,
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
	if existingPending != nil && !existingPending.tombstone {
		return finish(ref, true)
	}

	exists, err := s.inner.GetBlockExists(ctx, ref)
	if err != nil {
		return nil, false, err
	}

	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/enqueue")
	defer subtask.End()
	pendingClone := &pendingBlock{
		ref:  ref.Clone(),
		data: bytes.Clone(data),
		refs: CloneBlockRefs(opts.GetRefs()),
	}
	for {
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
				done = true
				return
			}
			if exists {
				alreadyExists = true
				done = true
				return
			}
			err := s.putPendingLocked(broadcastFn, key, pendingClone)
			if err == nil {
				done = true
				return
			}
			if err != ErrBufferedStoreFull {
				putErr = err
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

// PutBlockBatch loops through PutBlock and RmBlock using the buffered store.
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
		if _, _, err := s.PutBlock(ctx, entry.Data, &PutOpts{
			ForceBlockRef: ref,
			Refs:          CloneBlockRefs(entry.Refs),
		}); err != nil {
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
	pendingClone := &pendingBlock{
		ref:       ref.Clone(),
		tombstone: true,
	}
	for {
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
	var batch *drainBatch
	var drainErr error
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.drainErr != nil {
			drainErr = s.drainErr
			return
		}
		var subtask *trace.Task
		_, subtask = trace.NewTask(ctx, "hydra/block/buffered-store/drain/take-batch")
		batch = s.takeDrainBatchLocked()
		subtask.End()
	})
	if drainErr != nil {
		return false, drainErr
	}
	if batch == nil {
		return false, nil
	}

	writeCtx, writeTask := trace.NewTask(ctx, "hydra/block/buffered-store/drain/write-batch")
	err := s.writeBatch(writeCtx, batch.entries)
	writeTask.End()

	var returnErr error
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		if err != nil {
			s.returnDrainBatchLocked(batch)
			if ctx.Err() == nil {
				s.drainErr = err
			}
			broadcastFn()
			returnErr = err
			return
		}
		for _, key := range batch.keys {
			pending := s.pending[key]
			if pending == nil {
				continue
			}
			if pending.queued {
				continue
			}
			s.pendingBytes -= len(pending.data)
			delete(s.pending, key)
		}
		broadcastFn()
	})
	if returnErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return true, ctxErr
		}
		return true, returnErr
	}
	return true, nil
}

// takeDrainBatchLocked pops the next batch from the queue. Caller must
// hold bcast lock.
func (s *BufferedStore) takeDrainBatchLocked() *drainBatch {
	if len(s.queue) == 0 {
		return nil
	}

	keys := s.queue
	if s.drainBatchEntries > 0 && len(keys) > s.drainBatchEntries {
		keys = slices.Clone(keys[:s.drainBatchEntries])
		s.queue = s.queue[s.drainBatchEntries:]
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
		pending.queued = false
		batch.entries = append(batch.entries, &PutBatchEntry{
			Ref:       pending.ref.Clone(),
			Data:      pending.data,
			Refs:      CloneBlockRefs(pending.refs),
			Tombstone: pending.tombstone,
		})
	}
	return batch
}

// returnDrainBatchLocked requeues a failed batch in arrival order.
// Caller must hold bcast lock.
func (s *BufferedStore) returnDrainBatchLocked(batch *drainBatch) {
	if batch == nil || len(batch.keys) == 0 {
		return
	}
	requeue := make([]string, 0, len(batch.keys))
	for _, key := range batch.keys {
		pending := s.pending[key]
		if pending == nil {
			continue
		}
		if pending.queued {
			continue
		}
		pending.queued = true
		requeue = append(requeue, key)
	}
	if len(requeue) != 0 {
		s.queue = append(requeue, s.queue...)
	}
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
	prevBytes := 0
	if prev != nil {
		prevBytes = len(prev.data)
	}
	pendingBytes := 0
	if pending != nil {
		pendingBytes = len(pending.data)
	}
	nextBytes := s.pendingBytes - prevBytes + pendingBytes
	if prev == nil && s.maxPendingBlocks > 0 && len(s.pending) >= s.maxPendingBlocks {
		return ErrBufferedStoreFull
	}
	if s.maxPendingBytes > 0 && nextBytes > s.maxPendingBytes {
		return ErrBufferedStoreFull
	}

	pending.queued = prev == nil || !prev.queued
	s.pending[key] = pending
	s.pendingBytes = nextBytes
	if pending.queued {
		s.queue = append(s.queue, key)
		broadcastFn()
	}
	return nil
}

// _ is a type assertion.
var (
	_ StoreOps = (*BufferedStore)(nil)
)
