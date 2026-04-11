package block

import (
	"bytes"
	"context"
	"runtime/trace"
	"slices"
	"sync"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/util/routine"
)

type pendingBlock struct {
	ref       *BlockRef
	data      []byte
	seq       uint64
	tombstone bool
	queued    bool
}

type pendingRefRecord struct {
	source  *BlockRef
	targets []*BlockRef
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

	mtx               sync.Mutex
	pending           map[string]*pendingBlock
	pendingRefs       []pendingRefRecord
	pendingBytes      int
	maxPendingBytes   int
	maxPendingBlocks  int
	drainBatchEntries int

	queue      []string
	nextSeq    uint64
	durableSeq uint64
	waitCh     chan struct{}

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
		waitCh:            make(chan struct{}),
	}
	_, _ = s.rc.SetRoutine(s.drainLoop)
	_ = s.rc.SetContext(ctx, false)
	return s
}

// GetHashType returns the preferred hash type.
func (s *BufferedStore) GetHashType() hash.HashType {
	return s.inner.GetHashType()
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

	s.mtx.Lock()
	err = s.drainErr
	pending := s.pending[key]
	s.mtx.Unlock()
	if err != nil {
		return nil, false, err
	}
	if pending != nil && !pending.tombstone {
		return ref, true, nil
	}

	exists, err := s.inner.GetBlockExists(ctx, ref)
	if err != nil {
		return nil, false, err
	}

	s.mtx.Lock()
	if s.drainErr != nil {
		err := s.drainErr
		s.mtx.Unlock()
		return nil, false, err
	}
	if pending := s.pending[key]; pending != nil && !pending.tombstone {
		s.mtx.Unlock()
		return ref, true, nil
	}
	if exists {
		s.mtx.Unlock()
		return ref, true, nil
	}
	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/enqueue")
	if err := s.putPendingLocked(key, &pendingBlock{
		ref:  ref.Clone(),
		data: bytes.Clone(data),
	}); err != nil {
		subtask.End()
		s.mtx.Unlock()
		return nil, false, err
	}
	subtask.End()
	s.mtx.Unlock()
	return ref, false, nil
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

// RmBlock deletes a block by reference.
func (s *BufferedStore) RmBlock(ctx context.Context, ref *BlockRef) error {
	key, err := marshalRefKey(ref)
	if err != nil {
		return err
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
	err = s.drainErr
	pending := s.pending[key]
	if err != nil {
		return err
	}
	if pending != nil && pending.tombstone {
		return nil
	}
	return s.putPendingLocked(key, &pendingBlock{
		ref:       ref.Clone(),
		tombstone: true,
	})
}

// RecordBlockRefs buffers GC reference recording until Flush is called.
func (s *BufferedStore) RecordBlockRefs(_ context.Context, source *BlockRef, targets []*BlockRef) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	err := s.drainErr
	if err != nil {
		return err
	}
	s.pendingRefs = append(s.pendingRefs, pendingRefRecord{
		source:  source.Clone(),
		targets: cloneBlockRefs(targets),
	})
	return nil
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

// Flush waits for background block draining through the current fence, then
// forwards buffered ref-record operations into the wrapped store.
func (s *BufferedStore) Flush(ctx context.Context) error {
	_, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/flush/wait-durable")
	if err := s.waitForDurable(ctx); err != nil {
		subtask.End()
		return err
	}
	subtask.End()

	recorder, ok := s.inner.(BlockRefRecorder)
	if !ok {
		return nil
	}

	refs := s.takePendingRefs()
	if len(refs) == 0 {
		return nil
	}
	wrote, err := s.flushRefRecords(ctx, recorder, refs)
	if err != nil {
		s.restorePendingRefs(refs[wrote:])
		return err
	}
	return nil
}

// BeginDeferFlush forwards deferred-flush batching to the wrapped store.
func (s *BufferedStore) BeginDeferFlush() {
	if df, ok := s.inner.(DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush forwards deferred-flush batching to the wrapped store.
func (s *BufferedStore) EndDeferFlush(ctx context.Context) error {
	if df, ok := s.inner.(DeferFlushable); ok {
		return df.EndDeferFlush(ctx)
	}
	return nil
}

func (s *BufferedStore) wakeDrainersLocked() {
	if len(s.queue) == 0 {
		return
	}
	s.notifyLocked()
}

func (s *BufferedStore) drainLoop(ctx context.Context) error {
	for {
		s.mtx.Lock()
		err := s.drainErr
		if err != nil {
			s.mtx.Unlock()
			return err
		}
		taskCtx, subtask := trace.NewTask(ctx, "hydra/block/buffered-store/drain-loop/take-batch")
		batch := s.takeDrainBatchLocked()
		subtask.End()
		if batch == nil {
			waitCh := s.waitCh
			_, waitTask := trace.NewTask(ctx, "hydra/block/buffered-store/drain-loop/wait")
			s.mtx.Unlock()
			select {
			case <-ctx.Done():
				waitTask.End()
				return ctx.Err()
			case <-waitCh:
			}
			waitTask.End()
			continue
		}
		s.mtx.Unlock()

		writeCtx, writeTask := trace.NewTask(taskCtx, "hydra/block/buffered-store/drain-loop/write-batch")
		err = s.writeBatch(writeCtx, batch.entries)
		writeTask.End()

		s.mtx.Lock()
		if err != nil {
			s.queue = append(batch.keys, s.queue...)
			s.drainErr = err
			s.notifyLocked()
			s.mtx.Unlock()
			return err
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
		s.notifyLocked()
		s.mtx.Unlock()
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
	if batcher, ok := s.inner.(BatchPutStore); ok {
		batchCtx, batchTask := trace.NewTask(ctx, "hydra/block/buffered-store/write-batch/put-block-batch")
		err := batcher.PutBlockBatch(batchCtx, entries)
		batchTask.End()
		return err
	}
	for _, entry := range entries {
		if entry.Tombstone {
			rmCtx, rmTask := trace.NewTask(ctx, "hydra/block/buffered-store/write-batch/rm-block")
			if err := s.inner.RmBlock(rmCtx, entry.Ref.Clone()); err != nil {
				rmTask.End()
				return err
			}
			rmTask.End()
			continue
		}
		putCtx, putTask := trace.NewTask(ctx, "hydra/block/buffered-store/write-batch/put-block")
		if _, _, err := s.inner.PutBlock(putCtx, entry.Data, &PutOpts{
			ForceBlockRef: entry.Ref.Clone(),
		}); err != nil {
			putTask.End()
			return err
		}
		putTask.End()
	}
	return nil
}

func (s *BufferedStore) waitForDurable(ctx context.Context) error {
	for {
		s.mtx.Lock()
		err := s.drainErr
		targetSeq := s.nextSeq
		if err != nil {
			s.mtx.Unlock()
			return err
		}
		if s.durableSeq >= targetSeq {
			s.mtx.Unlock()
			return nil
		}
		waitCh := s.waitCh
		s.mtx.Unlock()

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

func (s *BufferedStore) takePendingRefs() []pendingRefRecord {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if len(s.pendingRefs) == 0 {
		return nil
	}
	refs := s.pendingRefs
	s.pendingRefs = nil
	return refs
}

func (s *BufferedStore) restorePendingRefs(refs []pendingRefRecord) {
	if len(refs) == 0 {
		return
	}
	s.mtx.Lock()
	s.pendingRefs = append(refs, s.pendingRefs...)
	s.mtx.Unlock()
}

func (s *BufferedStore) flushRefRecords(ctx context.Context, recorder BlockRefRecorder, refs []pendingRefRecord) (int, error) {
	for i, ref := range refs {
		if err := recorder.RecordBlockRefs(ctx, ref.source, ref.targets); err != nil {
			return i, err
		}
	}
	return len(refs), nil
}

func (s *BufferedStore) notifyLocked() {
	close(s.waitCh)
	s.waitCh = make(chan struct{})
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
	s.mtx.Lock()
	pending := s.pending[key]
	s.mtx.Unlock()
	return pending, nil
}

func (s *BufferedStore) putPendingLocked(key string, pending *pendingBlock) error {
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
		s.wakeDrainersLocked()
	}
	return nil
}

func lenPendingData(pending *pendingBlock) int {
	if pending == nil {
		return 0
	}
	return len(pending.data)
}

func cloneBlockRefs(refs []*BlockRef) []*BlockRef {
	if len(refs) == 0 {
		return nil
	}
	cloned := make([]*BlockRef, len(refs))
	for i, ref := range refs {
		if ref == nil {
			continue
		}
		cloned[i] = ref.Clone()
	}
	return cloned
}

// _ is a type assertion.
var (
	_ StoreOps         = (*BufferedStore)(nil)
	_ Flushable        = (*BufferedStore)(nil)
	_ DeferFlushable   = (*BufferedStore)(nil)
	_ BlockRefRecorder = (*BufferedStore)(nil)
)
