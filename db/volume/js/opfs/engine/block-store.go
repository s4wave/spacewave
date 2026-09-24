package engine

import (
	"bytes"
	"context"
	"maps"
	"runtime/trace"
	"sync"
	"time"

	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/workload"
	"github.com/s4wave/spacewave/net/hash"
)

const (
	// maxPendingBlocks bounds both queued entries and duplicate lookup metadata.
	maxPendingBlocks = 1024
	// maxPendingBytes bounds queued and in-flight payload bytes together.
	maxPendingBytes = 8 << 20
)

// BlockStore owns bounded local write admission and its durable pack store.
// A successful Sync covers every write admitted before that fence began.
type BlockStore struct {
	// raw owns immutable payload/index publication.
	raw *packStore
	// ctx cancels background and newly admitted work on volume shutdown.
	ctx context.Context
	// cancel stops maintenance before storage resources are released.
	cancel context.CancelFunc
	// done joins the single background worker.
	done chan struct{}
	// wake coalesces pending-write and maintenance notifications.
	wake chan struct{}
	// drain serializes durability fences and backpressure drains.
	drain csync.Mutex
	// mtx protects admission order and bounded local read-through state.
	mtx sync.Mutex
	// sequence orders admitted writes without retaining a history map.
	sequence uint64
	// queue retains arrival order, including entries currently being published.
	queue []*pendingWrite
	// pending points to the newest locally admitted value for each complete key.
	pending map[string]*pendingWrite
	// pendingBytes includes queued and in-flight bytes until publication succeeds.
	pendingBytes int
}

// pendingWrite is one immutable admitted operation in the local queue.
type pendingWrite struct {
	// sequence identifies its admission order for exact Sync fences.
	sequence uint64
	// key is the complete binary BlockRef identity.
	key string
	// entry owns the copied payload and reference until durable publication.
	entry *block.PutBatchEntry
}

// NewBlockStore starts bounded writeback and quiescent maintenance for a volume.
func NewBlockStore(ctx context.Context, e *Engine, hashType hash.HashType) *BlockStore {
	// Bind admission, writeback, and maintenance to the volume lifetime.
	ctx, cancel := context.WithCancel(ctx)
	s := &BlockStore{
		raw:     &packStore{engine: e, hashType: hashType},
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
		wake:    make(chan struct{}, 1),
		pending: make(map[string]*pendingWrite),
	}

	// Route engine cleanup notifications to the same background worker.
	e.mtx.Lock()
	e.wake = s.wake
	e.mtx.Unlock()
	go s.run()
	s.signal()
	return s
}

// Close cancels admission, joins writeback, and releases uncommitted local data.
func (s *BlockStore) Close() error {
	// Join background work before releasing any queued payloads.
	s.cancel()
	<-s.done
	release, err := s.drain.Lock(context.Background())
	if err != nil {
		return err
	}
	defer release()

	// Drop uncommitted admission state after all drainers have stopped.
	s.mtx.Lock()
	s.queue, s.pending = nil, nil
	s.pendingBytes = 0
	s.mtx.Unlock()
	return nil
}

// check rejects caller cancellation and use after the volume's lifetime ends.
func (s *BlockStore) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.ctx.Err() != nil {
		return ErrClosed
	}
	return nil
}

// signal schedules bounded writeback without blocking its caller.
func (s *BlockStore) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// run drains writes and continues bounded cleanup until the durable queue is idle.
func (s *BlockStore) run() {
	defer close(s.done)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		}
		for {
			// Coalesce arrivals and yield between maintenance quanta.
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			_, err := s.sync(s.ctx)
			cleaned, reclaimed := false, false
			if err == nil {
				cleaned, err = s.raw.engine.CleanPack(s.ctx)
			}
			if err == nil {
				reclaimed, err = s.raw.engine.Reclaim(s.ctx)
			}
			if err != nil {
				// Retry recoverable allocation or bridge failures without a hot loop.
				timer := time.NewTimer(time.Second)
				select {
				case <-s.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				continue
			}
			if !cleaned && !reclaimed {
				break
			}
		}
	}
}

// GetHashType returns the configured preferred hash without storage work.
func (s *BlockStore) GetHashType() hash.HashType {
	return s.raw.GetHashType()
}

// GetSupportedFeatures prevents redundant buffering above this volume.
func (s *BlockStore) GetSupportedFeatures() block.StoreFeature {
	return s.raw.GetSupportedFeatures() | block.StoreFeatureSelfBuffered
}

// PutBlock admits verified content and optionally completes its durability fence.
func (s *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Validate the caller's immutable content identity before admission.
	if err := s.check(ctx); err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}
	if len(data) > MaxValueBytes {
		return nil, false, ErrLimit
	}
	if opts == nil {
		opts = new(block.PutOpts)
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(s.GetHashType())
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forced := opts.GetForceBlockRef(); !forced.GetEmpty() && !ref.EqualsRef(forced) {
		return ref, false, block.ErrBlockRefMismatch
	}

	// Report pending duplicates; publication resolves durable duplicates.
	logBlock(ctx, workload.OpPut, 0, ref, int64(len(data)))
	existed, err := s.admit(ctx, &block.PutBatchEntry{Ref: ref, Data: data})
	if err == nil && opts.GetSync() {
		_, err = s.Sync(ctx)
	}
	return ref, existed, err
}

// admit orders copied writes and applies bounded backpressure before acceptance.
// Durable duplicates are resolved by packStore under its publication lock.
func (s *BlockStore) admit(ctx context.Context, entry *block.PutBatchEntry) (bool, error) {
	// Retain caller-owned bytes once, including across a capacity drain.
	keyBytes, err := blockKey(entry.Ref)
	if err != nil {
		return false, err
	}
	key := string(keyBytes)
	copied := &block.PutBatchEntry{Ref: entry.Ref.Clone(), Tombstone: entry.Tombstone}
	if !entry.Tombstone {
		copied.Data = bytes.Clone(entry.Data)
	}

	// Admit against only current local state; a batch needs no disk lookups.
	for {
		if err := s.check(ctx); err != nil {
			return false, err
		}
		s.mtx.Lock()
		if s.ctx.Err() != nil {
			s.mtx.Unlock()
			return false, ErrClosed
		}
		if pending := s.pending[key]; pending != nil && pending.entry.Tombstone == entry.Tombstone {
			s.mtx.Unlock()
			return !entry.Tombstone, nil
		}
		if len(s.queue) < maxPendingBlocks && s.pendingBytes+len(copied.Data) <= maxPendingBytes {
			s.sequence++
			write := &pendingWrite{sequence: s.sequence, key: key, entry: copied}
			s.queue = append(s.queue, write)
			s.pending[key] = write
			s.pendingBytes += len(copied.Data)
			s.mtx.Unlock()
			s.signal()
			return false, nil
		}
		s.mtx.Unlock()

		// Free bounded capacity through the existing durability fence.
		if _, err := s.sync(ctx); err != nil {
			return false, err
		}
	}
}

// PutBlockBatch validates supplied identities and admits the bounded local writes.
func (s *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	// Verify the whole batch before admitting any of its content.
	ctx, task := trace.NewTask(ctx, "hydra/opfs-engine/block-store/put-block-batch")
	defer task.End()
	var payloadBytes, tombstones int

	for _, entry := range entries {
		if entry == nil {
			return block.ErrEmptyBlockRef
		}
		if err := entry.Ref.Validate(false); err != nil {
			return err
		}
		payloadBytes += len(entry.Data)
		if entry.Tombstone {
			tombstones++
		}
		if !entry.Tombstone {
			if len(entry.Data) == 0 {
				return block.ErrEmptyBlock
			}
			if len(entry.Data) > MaxValueBytes {
				return ErrLimit
			}
			if err := entry.Ref.VerifyData(entry.Data, false); err != nil {
				return err
			}
		}
	}

	// Record the batch and its entries in order.
	trace.Logf(ctx, "hydra/opfs-engine/block-store/put-block-batch/shape", "entries=%d bytes=%d tombstones=%d", len(entries), payloadBytes, tombstones)
	if trace.IsEnabled() {
		batch := s.raw.engine.workloadIDs.Add(1)
		workload.Record{Op: workload.OpPutBatch, ID: batch, Size: int64(len(entries))}.Log(ctx)
		for _, entry := range entries {
			op := workload.OpPut
			if entry.Tombstone {
				op = workload.OpTombstone
			}
			logBlock(ctx, op, batch, entry.Ref, int64(len(entry.Data)))
		}
	}

	// Queue the verified entries without per-entry reads or publication waits.
	for _, entry := range entries {
		if _, err := s.admit(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// Sync publishes every write admitted before the captured sequence fence.
func (s *BlockStore) Sync(ctx context.Context) (bool, error) {
	workload.Record{Op: workload.OpSync}.Log(ctx)
	return s.sync(ctx)
}

// sync publishes the admitted prefix for callers, writeback, and backpressure.
func (s *BlockStore) sync(ctx context.Context) (bool, error) {
	// Reject closed stores and bind the fence to volume shutdown.
	if err := s.check(ctx); err != nil {
		return false, err
	}
	// Volume shutdown cancels caller-owned fences as well as background writeback.
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer func() { stop(); cancel() }()

	// Capture the admitted prefix before joining other durability fences.
	s.mtx.Lock()
	fence := s.sequence
	s.mtx.Unlock()
	release, err := s.drain.Lock(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	for {
		// Prepare one bounded pack from the captured prefix.
		if err := s.check(ctx); err != nil {
			return false, err
		}
		s.mtx.Lock()
		var batch []*pendingWrite
		var batchBytes int
		for _, pending := range s.queue {
			if pending.sequence > fence || len(batch) >= maxPackRecords {
				break
			}
			size := len(pending.entry.Data) + len(pending.key) + 12
			if len(batch) != 0 && batchBytes+size > maxPackBytes {
				break
			}
			batch = append(batch, pending)
			batchBytes += size
		}
		s.mtx.Unlock()
		if len(batch) == 0 {
			return true, nil
		}

		// Publish before releasing local read-through state or its capacity.
		entries := make([]*block.PutBatchEntry, len(batch))
		for i, pending := range batch {
			entries[i] = pending.entry
		}
		if err := s.raw.PutBlockBatch(ctx, entries); err != nil {
			return false, err
		}
		s.mtx.Lock()
		for _, pending := range batch {
			if s.pending[pending.key] == pending {
				delete(s.pending, pending.key)
			}
			s.pendingBytes -= len(pending.entry.Data)
		}
		clear(s.queue[:len(batch)])
		s.queue = s.queue[len(batch):]
		s.mtx.Unlock()
	}
}

// pendingEntry returns immutable local read-through state after lifetime checks.
func (s *BlockStore) pendingEntry(ctx context.Context, ref *block.BlockRef) (*block.PutBatchEntry, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	key, err := blockKey(ref)
	if err != nil {
		return nil, err
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if pending := s.pending[string(key)]; pending != nil {
		return pending.entry, nil
	}
	return nil, nil
}

// GetBlock reads admitted content before consulting durable immutable packs.
func (s *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.getBlock(ctx, ref, s.raw)
	logBlock(ctx, workload.OpGetBlock, 0, ref, foundSize(found, len(data)))
	return data, found, err
}

// getBlock preserves local visibility when a caller supplies a protected scope.
func (s *BlockStore) getBlock(ctx context.Context, ref *block.BlockRef, raw *packStore) ([]byte, bool, error) {
	pending, err := s.pendingEntry(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	if pending != nil {
		return bytes.Clone(pending.Data), !pending.Tombstone, nil
	}
	return raw.GetBlock(ctx, ref)
}

// GetBlockExists answers from pending state or the small location index.
func (s *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	stat, err := s.statBlock(ctx, ref, s.raw)
	logBlock(ctx, workload.OpBlockExists, 0, ref, presence(stat != nil))
	return stat != nil, err
}

// GetBlockExistsBatch shares one immutable generation for uncached references.
func (s *BlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Keep one durable snapshot and pending overlay for the complete lookup.
	read, release, err := s.beginRead(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	workload.Record{Op: workload.OpExistsBatch, ID: read.id, Size: int64(len(refs))}.Log(ctx)

	// Preserve input positions while skipping empty references.
	out := make([]bool, len(refs))
	for j, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		out[j], err = read.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// StatBlock returns pending or indexed payload length without reading payloads.
func (s *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	stat, err := s.statBlock(ctx, ref, s.raw)
	logBlock(ctx, workload.OpStatBlock, 0, ref, statSize(stat))
	return stat, err
}

// statBlock overlays admitted writes on the chosen protected durable scope.
func (s *BlockStore) statBlock(ctx context.Context, ref *block.BlockRef, raw *packStore) (*block.BlockStat, error) {
	pending, err := s.pendingEntry(ctx, ref)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		if pending.Tombstone {
			return nil, nil
		}
		return &block.BlockStat{Ref: ref.Clone(), Size: int64(len(pending.Data))}, nil
	}
	return raw.StatBlock(ctx, ref)
}

// RmBlock orders deletion after prior local writes and durably records its extent.
func (s *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	logBlock(ctx, workload.OpRm, 0, ref, 0)
	if _, err := s.admit(ctx, &block.PutBatchEntry{Ref: ref, Tombstone: true}); err != nil {
		return err
	}
	_, err := s.sync(ctx)
	return err
}

// BeginReadOperation pins durable files and retains local pending read-through.
func (s *BlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	read, release, err := s.beginRead(ctx)
	if err != nil {
		return nil, nil, err
	}
	workload.Record{Op: workload.OpReadBegin, ID: read.id}.Log(ctx)
	return read, func() {
		workload.Record{Op: workload.OpReadEnd, ID: read.id}.Log(ctx)
		release()
	}, nil
}

// beginRead opens a numbered read scope without recording its lifetime.
func (s *BlockStore) beginRead(ctx context.Context) (*scopedStore, func(), error) {
	if err := s.check(ctx); err != nil {
		return nil, nil, err
	}
	// Preserve admitted values if writeback drains while this operation is active.
	s.mtx.Lock()
	pending := maps.Clone(s.pending)
	s.mtx.Unlock()
	read, release, err := s.raw.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	scope := &scopedStore{owner: s, id: s.raw.engine.workloadIDs.Add(1), raw: read.(*packStore), pending: pending}
	return scope, release, nil
}

// _ verifies the public block storage contract.
var _ block.StoreOps = (*BlockStore)(nil)
