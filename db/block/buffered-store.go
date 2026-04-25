package block

import (
	"bytes"
	"context"
	"runtime/trace"
	"slices"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

type pendingBlock struct {
	ref       *BlockRef
	data      []byte
	refs      []*BlockRef
	seq       uint64
	tombstone bool
	queued    bool
}

type drainBatch struct {
	keys    []string
	entries []*PutBatchEntry
	lastSeq uint64
}

// BufferedStore buffers PutBlock calls in memory, drains them in the
// background, and exposes Flush as a durability fence.
type BufferedStore struct {
	inner StoreOps
	rc    *routine.RoutineContainer

	bcast             broadcast.Broadcast
	pending           map[string]*pendingBlock
	pendingBytes      int
	maxPendingBytes   int
	maxPendingBlocks  int
	drainBatchEntries int

	queue      []string
	nextSeq    uint64
	durableSeq uint64

	// deferFlush counts active defer-flush scopes.
	deferFlush atomic.Int64
	// drainErr captures the last drain error to surface on subsequent calls.
	drainErr error
}

// NewBufferedStore constructs a buffered store around an inner
// store and uses ctx for background draining.
func NewBufferedStore(ctx context.Context, inner StoreOps) *BufferedStore {
	return NewBufferedStoreWithSettings(ctx, inner, nil)
}

// NewBufferedStoreWithSettings constructs a buffered store with explicit settings.
func NewBufferedStoreWithSettings(
	ctx context.Context,
	inner StoreOps,
	settings *BufferedStoreSettings,
) *BufferedStore {
	if ctx == nil {
		ctx = context.Background()
	}
	settings = normalizeBufferedStoreSettings(settings)
	s := &BufferedStore{
		inner:             inner,
		rc:                routine.NewRoutineContainer(),
		pending:           make(map[string]*pendingBlock),
		maxPendingBytes:   settings.MaxPendingBytes,
		maxPendingBlocks:  settings.MaxPendingEntries,
		drainBatchEntries: settings.DrainBatchEntries,
	}
	_, _ = s.rc.SetRoutine(s.drainLoop)
	_ = s.rc.SetContext(ctx, false)
	return s
}

// GetHashType returns the preferred hash type.
func (s *BufferedStore) GetHashType() hash.HashType {
	return s.inner.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *BufferedStore) GetSupportedFeatures() StoreFeature {
	return s.inner.GetSupportedFeatures() | StoreFeatureNativeFlush | StoreFeatureNativeDeferFlush
}

// PutBlock buffers a block in memory and starts background draining if needed.
func (s *BufferedStore) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	if len(data) == 0 {
		return nil, false, ErrEmptyBlock
	}

	if opts == nil {
		opts = &PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(s.inner.GetHashType())

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
		return ref, true, nil
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
		var waitCh <-chan struct{}
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
			waitCh = getWaitCh()
		})
		if done {
			if putErr != nil {
				return nil, false, putErr
			}
			if alreadyExists {
				return ref, true, nil
			}
			return ref, false, nil
		}
		_, waitTask := trace.NewTask(ctx, "hydra/block/buffered-store/enqueue/wait-capacity")
		select {
		case <-ctx.Done():
			waitTask.End()
			return nil, false, ctx.Err()
		case <-waitCh:
		}
		waitTask.End()
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
		var waitCh <-chan struct{}
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
			waitCh = getWaitCh()
		})
		if done {
			return rmErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// PutBlockBackground forwards to PutBlock because buffered writes already drain asynchronously.
func (s *BufferedStore) PutBlockBackground(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return s.PutBlock(ctx, data, opts)
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

// Flush waits for background block draining through the current fence.
func (s *BufferedStore) Flush(ctx context.Context) error {
	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/flush/wait-durable")
	if err := s.waitForDurable(ctx); err != nil {
		subtask.End()
		return err
	}
	subtask.End()
	return nil
}

// BeginDeferFlush opens a nested deferred flush scope.
func (s *BufferedStore) BeginDeferFlush() {
	s.deferFlush.Add(1)
	s.inner.BeginDeferFlush()
}

// EndDeferFlush closes a deferred flush scope and flushes at the outermost end.
func (s *BufferedStore) EndDeferFlush(ctx context.Context) error {
	depth := s.deferFlush.Add(-1)
	if depth < 0 {
		return errors.New("block: EndDeferFlush called more than BeginDeferFlush")
	}
	innerErr := s.inner.EndDeferFlush(ctx)
	if depth != 0 {
		return innerErr
	}
	if err := s.Flush(ctx); err != nil {
		return err
	}
	return innerErr
}

func (s *BufferedStore) drainLoop(ctx context.Context) error {
	for {
		var batch *drainBatch
		var waitCh <-chan struct{}
		var loopErr error
		var batchCtx context.Context
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if s.drainErr != nil {
				loopErr = s.drainErr
				return
			}
			var subtask *trace.Task
			batchCtx, subtask = trace.NewTask(ctx, "hydra/block/buffered-store/drain-loop/take-batch")
			batch = s.takeDrainBatchLocked()
			subtask.End()
			if batch == nil {
				waitCh = getWaitCh()
			}
		})
		if loopErr != nil {
			return loopErr
		}
		if batch == nil {
			_, waitTask := trace.NewTask(ctx, "hydra/block/buffered-store/drain-loop/wait")
			select {
			case <-ctx.Done():
				waitTask.End()
				return ctx.Err()
			case <-waitCh:
			}
			waitTask.End()
			continue
		}

		writeCtx, writeTask := trace.NewTask(batchCtx, "hydra/block/buffered-store/drain-loop/write-batch")
		err := s.writeBatch(writeCtx, batch.entries)
		writeTask.End()

		var returnErr error
		s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
			if err != nil {
				s.queue = append(batch.keys, s.queue...)
				s.drainErr = err
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
			if batch.lastSeq > s.durableSeq {
				s.durableSeq = batch.lastSeq
			}
			broadcastFn()
		})
		if returnErr != nil {
			return returnErr
		}
	}
}

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
		if pending.seq > batch.lastSeq {
			batch.lastSeq = pending.seq
		}
	}
	return batch
}

func (s *BufferedStore) writeBatch(ctx context.Context, entries []*PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	batchCtx, batchTask := trace.NewTask(ctx, "hydra/block/buffered-store/write-batch/put-block-batch")
	err := s.inner.PutBlockBatch(batchCtx, entries)
	batchTask.End()
	return err
}

func (s *BufferedStore) waitForDurable(ctx context.Context) error {
	for {
		var waitCh <-chan struct{}
		var done bool
		var waitErr error
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if s.drainErr != nil {
				waitErr = s.drainErr
				done = true
				return
			}
			if s.durableSeq >= s.nextSeq {
				done = true
				return
			}
			waitCh = getWaitCh()
		})
		if done {
			return waitErr
		}

		_, waitTask := trace.NewTask(ctx, "hydra/block/buffered-store/flush/wait-durable/wait-notify")
		select {
		case <-ctx.Done():
			waitTask.End()
			return ctx.Err()
		case <-waitCh:
		}
		waitTask.End()
	}
}

func marshalRefKey(ref *BlockRef) (string, error) {
	dat, err := ref.MarshalKey()
	if err != nil {
		return "", err
	}
	return string(dat), nil
}

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

func (s *BufferedStore) putPendingLocked(broadcastFn func(), key string, pending *pendingBlock) error {
	prev := s.pending[key]
	nextBytes := s.pendingBytes - lenPendingData(prev) + lenPendingData(pending)
	if prev == nil && s.maxPendingBlocks > 0 && len(s.pending) >= s.maxPendingBlocks {
		return ErrBufferedStoreFull
	}
	if s.maxPendingBytes > 0 && nextBytes > s.maxPendingBytes {
		return ErrBufferedStoreFull
	}

	s.nextSeq++
	pending.seq = s.nextSeq
	pending.queued = prev == nil || !prev.queued
	s.pending[key] = pending
	s.pendingBytes = nextBytes
	if pending.queued {
		s.queue = append(s.queue, key)
		broadcastFn()
	}
	return nil
}

func lenPendingData(pending *pendingBlock) int {
	if pending == nil {
		return 0
	}
	return len(pending.data)
}

// _ is a type assertion.
var (
	_ StoreOps = (*BufferedStore)(nil)
)
