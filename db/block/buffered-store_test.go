package block

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/hash"
)

type countStore struct {
	NopStoreOps

	hashType hash.HashType

	mtx           sync.Mutex
	blocks        map[string][]byte
	putCalls      int
	existsCalls   int
	batchCalls    int
	batchSizes    []int
	failPut       error
	recordCalls   int
	recordFailAt  int
	recordErr     error
	recordTargets map[string]int

	batchStarted chan struct{}
	batchRelease chan struct{}
}

type syncOrderStore struct {
	*countStore

	eventMtx   sync.Mutex
	events     []string
	syncCalled chan struct{}
	syncOnce   sync.Once
}

type signalDoneContext struct {
	context.Context

	once     sync.Once
	observed chan struct{}
}

func (c *signalDoneContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.observed)
	})
	return c.Context.Done()
}

func newCountStore(hashType hash.HashType) *countStore {
	return &countStore{
		hashType:      hashType,
		blocks:        make(map[string][]byte),
		recordTargets: make(map[string]int),
	}
}

func newSyncOrderStore(hashType hash.HashType) *syncOrderStore {
	return &syncOrderStore{
		countStore: newCountStore(hashType),
		syncCalled: make(chan struct{}),
	}
}

func (s *syncOrderStore) appendEvent(event string) {
	s.eventMtx.Lock()
	s.events = append(s.events, event)
	s.eventMtx.Unlock()
}

func (s *syncOrderStore) snapshotEvents() []string {
	s.eventMtx.Lock()
	defer s.eventMtx.Unlock()
	return slices.Clone(s.events)
}

func (s *syncOrderStore) Sync(ctx context.Context) (bool, error) {
	s.appendEvent("sync")
	s.syncOnce.Do(func() { close(s.syncCalled) })
	return s.countStore.Sync(ctx)
}

func (s *syncOrderStore) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	s.appendEvent("put-start")
	err := s.countStore.PutBlockBatch(ctx, entries)
	s.appendEvent("put-done")
	return err
}

func (s *countStore) GetHashType() hash.HashType {
	return s.hashType
}

func (s *countStore) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	// Count the test-store write and honor its configured failure.
	s.mtx.Lock()
	s.putCalls++
	failPut := s.failPut
	s.mtx.Unlock()
	if failPut != nil {
		return nil, false, failPut
	}

	// Copy the put options and derive the stored block reference.
	if opts == nil {
		opts = &PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(s.hashType)
	ref, err := BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, false, err
	}

	// Retain the block bytes and outgoing references under the store lock.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	_, exists := s.blocks[key]
	if !exists {
		s.blocks[key] = bytes.Clone(data)
	}
	s.recordRefTargetsLocked(ref, opts.GetRefs())
	return ref, exists, nil
}

func (s *countStore) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	// Record the test batch and capture its gate and failure settings.
	s.mtx.Lock()
	s.batchCalls++
	s.batchSizes = append(s.batchSizes, len(entries))
	started := s.batchStarted
	release := s.batchRelease
	failPut := s.failPut
	s.mtx.Unlock()

	// Signal batch entry and wait for the test-controlled release.
	if started != nil {
		select {
		case <-started:
		default:
			close(started)
		}
	}
	if release != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
		}
	}
	if failPut != nil {
		return failPut
	}

	// Apply batch tombstones and new blocks under the store lock.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, entry := range entries {
		key, err := marshalRefKey(entry.Ref)
		if err != nil {
			return err
		}
		if entry.Tombstone {
			delete(s.blocks, key)
			continue
		}
		if _, exists := s.blocks[key]; exists {
			continue
		}
		s.blocks[key] = bytes.Clone(entry.Data)
		s.recordRefTargetsLocked(entry.Ref, entry.Refs)
	}
	return nil
}

func (s *countStore) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	// Look up the block bytes under the test-store lock.
	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, false, err
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	data, ok := s.blocks[key]
	if !ok {
		return nil, false, nil
	}
	return bytes.Clone(data), true, nil
}

func (s *countStore) GetBlockExists(ctx context.Context, ref *BlockRef) (bool, error) {
	// Count the existence probe and look up the block reference.
	key, err := marshalRefKey(ref)
	if err != nil {
		return false, err
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.existsCalls++
	_, ok := s.blocks[key]
	return ok, nil
}

func (s *countStore) RmBlock(ctx context.Context, ref *BlockRef) error {
	// Remove the block bytes under the test-store lock.
	key, err := marshalRefKey(ref)
	if err != nil {
		return err
	}
	s.mtx.Lock()
	delete(s.blocks, key)
	s.mtx.Unlock()
	return nil
}

func (s *countStore) StatBlock(ctx context.Context, ref *BlockRef) (*BlockStat, error) {
	// Read the stored block size under the test-store lock.
	key, err := marshalRefKey(ref)
	if err != nil {
		return nil, err
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	data, ok := s.blocks[key]
	if !ok {
		return nil, nil
	}
	return &BlockStat{
		Ref:  ref.Clone(),
		Size: int64(len(data)),
	}, nil
}

func (s *countStore) recordRefTargetsLocked(source *BlockRef, targets []*BlockRef) {
	// Record outgoing targets only while the test store accepts them.
	if len(targets) == 0 {
		return
	}
	if s.recordErr != nil && s.recordFailAt > 0 && s.recordCalls >= s.recordFailAt {
		return
	}
	s.recordCalls++
	key, err := marshalRefKey(source)
	if err != nil {
		return
	}
	s.recordTargets[key] += len(targets)
}

func (s *countStore) setBatchBlocker() <-chan struct{} {
	// Install a batch gate for the test store.
	started := make(chan struct{})
	s.mtx.Lock()
	s.batchStarted = started
	s.batchRelease = make(chan struct{})
	s.mtx.Unlock()
	return started
}

func (s *countStore) releaseBatchBlocker() {
	// Release the captured batch gate after clearing store state.
	s.mtx.Lock()
	release := s.batchRelease
	s.batchRelease = nil
	s.batchStarted = nil
	s.mtx.Unlock()
	if release != nil {
		close(release)
	}
}

func waitSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func TestBufferedStoreKeepsPendingUntilFlush(t *testing.T) {
	// Create a buffered store with an empty backing store.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	// Queue a new block in the buffered store.
	ref, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected buffered put to be new")
	}

	// Verify the backing store has not received the pending block.
	found, err := inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected buffered put to stay pending before flush")
	}

	// Verify the buffered store exposes the pending block.
	found, err = store.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected buffered store to read through pending block")
	}

	// Flush the pending block and verify backing-store durability.
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
	found, err = inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected block to be durable after flush")
	}
}

func TestBufferedStoreFlushWaitsForDurableDrain(t *testing.T) {
	// Create a buffered store whose backing batch waits on a gate.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	// Queue the block whose drain will be gated.
	ref, _, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the durability fence and wait for the backing batch.
	errCh := make(chan error, 1)
	go func() {
		_, err := store.Sync(ctx)
		errCh <- err
	}()
	waitSignal(t, started, "flush drain")

	// Verify the fence remains blocked while the batch is gated.
	select {
	case err := <-errCh:
		t.Fatalf("flush returned early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// Release the backing batch and verify the durability fence completes.
	inner.releaseBatchBlocker()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for flush to finish")
	}

	// Verify the drained block reached the backing store.
	found, err := inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected block to be durable after flush")
	}
}

func TestBufferedStoreDedupsPendingBlock(t *testing.T) {
	// Create an empty buffered store for duplicate writes.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	// Queue identical blocks and verify they share one reference.
	ref1, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected first buffered put to be new")
	}
	ref2, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("expected second buffered put to report exists")
	}
	if !ref1.EqualsRef(ref2) {
		t.Fatal("expected duplicate buffered put to return same ref")
	}

	// Flush the duplicates and verify a single batch write.
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.batchCalls != 1 {
		t.Fatalf("expected one batch after deduped flush, got %d", inner.batchCalls)
	}
	if inner.putCalls != 0 {
		t.Fatalf("expected batch path instead of serial PutBlock, got %d single puts", inner.putCalls)
	}
}

func TestBufferedStoreReadsThroughPendingBlock(t *testing.T) {
	// Create a buffered store for pending-block readback.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	// Queue a block before reading it through the buffer.
	ref, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected buffered put to be new")
	}

	// Verify the pending block bytes are readable.
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected pending block to be visible to GetBlock")
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected pending block data: %q", string(data))
	}

	// Verify the pending block passes the existence probe.
	found, err = store.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected pending block to be visible to GetBlockExists")
	}

	// Verify the pending block reports its payload size.
	stat, err := store.StatBlock(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stat == nil {
		t.Fatal("expected pending block stat")
	}
	if stat.Size != 5 {
		t.Fatalf("unexpected pending stat size: %d", stat.Size)
	}

	// Flush the block after pending readback checks.
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestBufferedStoreReportsDrainErrorAtFlush(t *testing.T) {
	// Configure the backing store to reject draining writes.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	inner.failPut = context.DeadlineExceeded
	store := NewBufferedStore(ctx, inner)

	// Queue a block before the failing drain.
	ref, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected buffered put to be new")
	}

	// Verify the failed fence leaves the backing store empty.
	if _, err := store.Sync(ctx); err == nil {
		t.Fatal("expected flush error")
	}
	found, err := inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected failed drain to avoid persistence")
	}

	// Verify the buffer rejects writes after its drain failure.
	_, _, err = store.PutBlock(ctx, []byte("again"), nil)
	if err == nil {
		t.Fatal("expected buffered store to reject new writes after drain failure")
	}
}

func TestBufferedStoreFlushesBufferedPutRefs(t *testing.T) {
	// Create an empty buffer for referenced block writes.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	// Queue the source and target without recording backing-store references.
	dst, _, err := store.PutBlock(ctx, []byte("dst"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	src, _, err := store.PutBlock(ctx, []byte("src"), &PutOpts{Refs: []*BlockRef{dst}})
	if err != nil {
		t.Fatal(err.Error())
	}
	if inner.recordCalls != 0 {
		t.Fatalf("expected no inner ref recording before flush, got %d", inner.recordCalls)
	}

	// Flush the blocks and verify the outgoing target was recorded.
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.recordCalls != 1 {
		t.Fatalf("expected one inner ref recording after flush, got %d", inner.recordCalls)
	}
	key, err := marshalRefKey(src)
	if err != nil {
		t.Fatal(err.Error())
	}
	if inner.recordTargets[key] != 1 {
		t.Fatalf("expected one recorded target after flush, got %d", inner.recordTargets[key])
	}
}

// TestBufferedStoreSyncDrainsBeforeInnerSync proves Sync drains all buffered
// blocks into the inner store before forwarding the inner durability barrier.
// The nested-defer-flush ref-batch counting invariant is owned by GCStoreOps
// and covered by TestGCStoreOps_NestedDeferFlushFlushesOnce.
func TestBufferedStoreSyncDrainsBeforeInnerSync(t *testing.T) {
	// Create a gated backing store that records durability events.
	ctx := context.Background()
	inner := newSyncOrderStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	// Queue the block whose drain must precede the inner fence.
	if _, _, err := store.PutBlock(ctx, []byte("src"), nil); err != nil {
		t.Fatal(err.Error())
	}

	// Start the buffer fence and wait for its backing batch.
	errCh := make(chan error, 1)
	go func() {
		_, err := store.Sync(ctx)
		errCh <- err
	}()
	waitSignal(t, started, "sync drain")

	// Verify the inner fence has not run during the gated drain.
	select {
	case <-inner.syncCalled:
		t.Fatalf("inner Sync ran before buffered writes drained: %v", inner.snapshotEvents())
	case <-time.After(25 * time.Millisecond):
	}

	// Release the drain and wait for the buffer fence.
	inner.releaseBatchBlocker()
	if err := <-errCh; err != nil {
		t.Fatal(err.Error())
	}

	// Verify the inner fence follows the completed backing batch.
	events := inner.snapshotEvents()
	putDone := slices.Index(events, "put-done")
	syncIdx := slices.Index(events, "sync")
	if putDone == -1 || syncIdx == -1 {
		t.Fatalf("missing expected events: %v", events)
	}
	if syncIdx < putDone {
		t.Fatalf("inner Sync ran before buffered writes drained: %v", events)
	}
}

func TestBufferedStoreBlocksWhenPendingLimitExceeded(t *testing.T) {
	// Create a gated buffer with capacity for one pending block.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)
	store.maxPendingBlocks = 1
	store.maxPendingBytes = 4

	// Fill the pending-block capacity with the first write.
	_, exists, err := store.PutBlock(ctx, []byte("one"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected first buffered put to be new")
	}

	// Second PutBlock should block until the first drains instead of returning
	// ErrBufferedStoreFull.
	type putResult struct {
		exists bool
		err    error
	}
	done := make(chan putResult, 1)
	go func() {
		_, exists, err := store.PutBlock(ctx, []byte("two"), nil)
		done <- putResult{exists: exists, err: err}
	}()
	waitSignal(t, started, "capacity drain")

	// Verify the second put waits while the capacity drain is gated.
	select {
	case res := <-done:
		t.Fatalf("second put returned before drain: exists=%v err=%v", res.exists, res.err)
	case <-time.After(50 * time.Millisecond):
	}

	// Release the capacity drain and verify the blocked put completes.
	inner.releaseBatchBlocker()
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("second put failed: %v", res.err)
		}
		if res.exists {
			t.Fatal("expected second buffered put to be new")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second put did not unblock after drain release")
	}

	// Flush the remaining buffered write.
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestBufferedStoreCapacityDrainerRetriesAfterConcurrentSync(t *testing.T) {
	// Build the reference used by the capacity removal case.
	ctx := context.Background()
	refTwo, err := BuildBlockRef([]byte("two"), &PutOpts{
		HashType: hash.HashType_HashType_BLAKE3,
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Define the writes that compete with a concurrent fence.
	ops := []struct {
		name string
		run  func(context.Context, *BufferedStore) error
	}{
		{
			name: "put",
			run: func(ctx context.Context, store *BufferedStore) error {
				_, _, err := store.PutBlock(ctx, []byte("two"), nil)
				return err
			},
		},
		{
			name: "remove",
			run: func(ctx context.Context, store *BufferedStore) error {
				return store.RmBlock(ctx, refTwo)
			},
		},
	}

	// Exercise each capacity operation against a concurrent drain.
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			// Fill a buffer that can retain only one pending block.
			inner := newCountStore(hash.HashType_HashType_BLAKE3)
			store := NewBufferedStoreWithSettings(ctx, inner, &BufferedStoreSettings{
				MaxPendingEntries: 1,
				MaxPendingBytes:   4,
			})
			if _, _, err := store.PutBlock(ctx, []byte("one"), nil); err != nil {
				t.Fatal(err.Error())
			}

			// Gate the backing batch and start a concurrent fence.
			started := inner.setBatchBlocker()
			t.Cleanup(inner.releaseBatchBlocker)
			syncDone := make(chan error, 1)
			go func() {
				_, err := store.Sync(ctx)
				syncDone <- err
			}()
			waitSignal(t, started, "sync capacity drain")

			// Start a competing operation and observe its capacity wait.
			opCtx, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)
			observed := make(chan struct{})
			waitCtx := &signalDoneContext{
				Context:  opCtx,
				observed: observed,
			}
			opDone := make(chan error, 1)
			go func() {
				opDone <- op.run(waitCtx, store)
			}()
			waitSignal(t, observed, "capacity drainer contention")

			// Release the drain and verify both competing operations finish.
			inner.releaseBatchBlocker()
			select {
			case err := <-syncDone:
				if err != nil {
					t.Fatal(err.Error())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("concurrent sync did not complete")
			}
			select {
			case err := <-opDone:
				if err != nil {
					t.Fatalf("%s after concurrent drain: %v", op.name, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s did not retry after concurrent drain", op.name)
			}
			if _, err := store.Sync(ctx); err != nil {
				t.Fatal(err.Error())
			}
		})
	}
}

func TestBufferedStoreUnblocksOnContextCancel(t *testing.T) {
	// Create a gated buffer with one-block capacity.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)
	store.maxPendingBlocks = 1
	store.maxPendingBytes = 4

	// Fill the buffer before starting a cancelable write.
	if _, _, err := store.PutBlock(ctx, []byte("one"), nil); err != nil {
		t.Fatal(err.Error())
	}

	// Start the second put and wait for its capacity drain.
	cancelCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, _, err := store.PutBlock(cancelCtx, []byte("two"), nil)
		done <- err
	}()
	waitSignal(t, started, "capacity drain")

	// Verify the second put is blocked before cancellation.
	select {
	case err := <-done:
		t.Fatalf("blocked put returned prematurely: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// Cancel the blocked put and verify it releases its capacity wait.
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked put did not return after context cancel")
	}

	// Release the backing batch and flush remaining writes.
	inner.releaseBatchBlocker()
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestBufferedStoreUsesBatchPut(t *testing.T) {
	// Create a buffered store for a two-block batch.
	ctx := t.Context()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	// Queue two blocks without serial existence probes.
	if err := store.PutBlockBatch(ctx, []*PutBatchEntry{
		{Data: []byte("a")},
		{Data: []byte("b")},
	}); err != nil {
		t.Fatal(err)
	}
	if inner.existsCalls != 0 {
		t.Fatalf("batch performed %d serial existence probes", inner.existsCalls)
	}

	// Flush the batch and verify both blocks used the batch path.
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.batchCalls == 0 {
		t.Fatal("expected batch call")
	}
	var batchedBlocks int
	for _, size := range inner.batchSizes {
		batchedBlocks += size
	}
	if batchedBlocks != 2 {
		t.Fatalf("expected two batched blocks, got batch sizes %v", inner.batchSizes)
	}
	if inner.putCalls != 0 {
		t.Fatalf("expected no serial PutBlock fallback calls, got %d", inner.putCalls)
	}
}

func TestBufferedStoreRemovesPendingBlockWithoutResurrection(t *testing.T) {
	// Create an empty buffer for block removal.
	ctx := t.Context()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	// Queue the block that will be removed.
	ref, _, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Queue a tombstone for the pending block.
	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the pending tombstone hides the block.
	found, err := store.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected pending tombstone to hide block")
	}

	// Flush the buffered tombstone.
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the backing store retains no removed block.
	found, err = inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected tombstone to win over pending put")
	}

	// A later put must replace a queued deletion even while the old block exists.
	if _, _, err := inner.PutBlock(ctx, []byte("hello"), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PutBlock(ctx, []byte("hello"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if data, found, err := inner.GetBlock(ctx, ref); err != nil || !found || string(data) != "hello" {
		t.Fatalf("put after queued deletion: data=%q found=%t err=%v", data, found, err)
	}
}

func TestBufferedStoreFlushContextCancelCanRetry(t *testing.T) {
	// Create a gated buffer for a cancelable durability fence.
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	// Queue the block whose fence will be canceled.
	if _, _, err := store.PutBlock(ctx, []byte("hello"), nil); err != nil {
		t.Fatal(err.Error())
	}

	// Start the cancelable fence and wait for its backing drain.
	cancelCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		_, err := store.Sync(cancelCtx)
		errCh <- err
	}()
	waitSignal(t, started, "flush drain")

	// Cancel the durability fence and verify its cancellation result.
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("flush did not return after context cancel")
	}

	// Release the batch and verify a subsequent fence succeeds.
	inner.releaseBatchBlocker()
	if _, err := store.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
}
