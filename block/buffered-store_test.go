package block

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/bifrost/hash"
)

type countStore struct {
	hashType hash.HashType

	mtx           sync.Mutex
	blocks        map[string][]byte
	putCalls      int
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

func newCountStore(hashType hash.HashType) *countStore {
	return &countStore{
		hashType:      hashType,
		blocks:        make(map[string][]byte),
		recordTargets: make(map[string]int),
	}
}

func (s *countStore) GetHashType() hash.HashType {
	return s.hashType
}

func (s *countStore) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	s.mtx.Lock()
	s.putCalls++
	failPut := s.failPut
	s.mtx.Unlock()
	if failPut != nil {
		return nil, false, failPut
	}
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
	s.mtx.Lock()
	defer s.mtx.Unlock()
	_, exists := s.blocks[key]
	if !exists {
		s.blocks[key] = bytes.Clone(data)
	}
	return ref, exists, nil
}

func (s *countStore) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	s.mtx.Lock()
	s.batchCalls++
	s.batchSizes = append(s.batchSizes, len(entries))
	started := s.batchStarted
	release := s.batchRelease
	failPut := s.failPut
	s.mtx.Unlock()

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
	}
	return nil
}

func (s *countStore) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
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
	key, err := marshalRefKey(ref)
	if err != nil {
		return false, err
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	_, ok := s.blocks[key]
	return ok, nil
}

func (s *countStore) RmBlock(ctx context.Context, ref *BlockRef) error {
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

func (s *countStore) RecordBlockRefs(ctx context.Context, source *BlockRef, targets []*BlockRef) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.recordErr != nil && s.recordFailAt > 0 && s.recordCalls >= s.recordFailAt {
		return s.recordErr
	}
	s.recordCalls++
	key, err := marshalRefKey(source)
	if err != nil {
		return err
	}
	s.recordTargets[key] += len(targets)
	return nil
}

func (s *countStore) setBatchBlocker() <-chan struct{} {
	started := make(chan struct{})
	s.mtx.Lock()
	s.batchStarted = started
	s.batchRelease = make(chan struct{})
	s.mtx.Unlock()
	return started
}

func (s *countStore) releaseBatchBlocker() {
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

func TestBufferedStoreStartsBackgroundDrainBeforeFlush(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	ref, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected buffered put to be new")
	}
	waitSignal(t, started, "background drain")

	found, err := inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected blocked background drain to keep block pending")
	}

	inner.releaseBatchBlocker()
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestBufferedStoreFlushWaitsForDurableDrain(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	ref, _, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	waitSignal(t, started, "background drain")

	errCh := make(chan error, 1)
	go func() {
		errCh <- store.Flush(ctx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("flush returned early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	inner.releaseBatchBlocker()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for flush to finish")
	}

	found, err := inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected block to be durable after flush")
	}
}

func TestBufferedStoreDedupsPendingBlock(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

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

	if err := store.Flush(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.batchCalls != 1 {
		t.Fatalf("expected one background batch after deduped flush, got %d", inner.batchCalls)
	}
	if inner.putCalls != 0 {
		t.Fatalf("expected batch path instead of serial PutBlock, got %d single puts", inner.putCalls)
	}
}

func TestBufferedStoreReadsThroughPendingBlock(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	ref, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected buffered put to be new")
	}
	waitSignal(t, started, "background drain")

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

	found, err = store.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected pending block to be visible to GetBlockExists")
	}

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

	inner.releaseBatchBlocker()
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestBufferedStoreReportsDrainErrorAtFlush(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	inner.failPut = context.DeadlineExceeded
	store := NewBufferedStore(ctx, inner)

	ref, exists, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected buffered put to be new")
	}

	if err := store.Flush(ctx); err == nil {
		t.Fatal("expected flush error")
	}
	found, err := inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected failed drain to avoid persistence")
	}

	_, _, err = store.PutBlock(ctx, []byte("again"), nil)
	if err == nil {
		t.Fatal("expected buffered store to reject new writes after drain failure")
	}
}

func TestBufferedStoreFlushesBufferedRefRecords(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	src, _, err := store.PutBlock(ctx, []byte("src"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	dst, _, err := store.PutBlock(ctx, []byte("dst"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := store.RecordBlockRefs(ctx, src, []*BlockRef{dst}); err != nil {
		t.Fatal(err.Error())
	}
	if inner.recordCalls != 0 {
		t.Fatalf("expected no inner ref recording before flush, got %d", inner.recordCalls)
	}

	if err := store.Flush(ctx); err != nil {
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

func TestBufferedStoreReturnsFullWhenPendingLimitExceeded(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)
	store.maxPendingBlocks = 1
	store.maxPendingBytes = 4

	_, exists, err := store.PutBlock(ctx, []byte("one"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("expected first buffered put to be new")
	}
	waitSignal(t, started, "background drain")

	_, _, err = store.PutBlock(ctx, []byte("two"), nil)
	if err != ErrBufferedStoreFull {
		t.Fatalf("expected ErrBufferedStoreFull, got %v", err)
	}

	inner.releaseBatchBlocker()
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestBufferedStoreUsesBatchPutStore(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	store := NewBufferedStore(ctx, inner)

	if _, _, err := store.PutBlock(ctx, []byte("a"), nil); err != nil {
		t.Fatal(err.Error())
	}
	if _, _, err := store.PutBlock(ctx, []byte("b"), nil); err != nil {
		t.Fatal(err.Error())
	}

	if err := store.Flush(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if inner.batchCalls != 1 {
		t.Fatalf("expected one batch call, got %d", inner.batchCalls)
	}
	if len(inner.batchSizes) != 1 || inner.batchSizes[0] != 2 {
		t.Fatalf("expected one batch of two blocks, got %v", inner.batchSizes)
	}
	if inner.putCalls != 0 {
		t.Fatalf("expected no serial PutBlock fallback calls, got %d", inner.putCalls)
	}
}

func TestBufferedStoreRemovesPendingBlockWithoutResurrection(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	ref, _, err := store.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	waitSignal(t, started, "background drain")

	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}

	found, err := store.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected pending tombstone to hide block")
	}

	inner.releaseBatchBlocker()
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err.Error())
	}

	found, err = inner.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatal("expected tombstone to win over pending put")
	}
}

func TestBufferedStoreDrainUsesStoreContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	started := inner.setBatchBlocker()
	store := NewBufferedStore(ctx, inner)

	if _, _, err := store.PutBlock(context.Background(), []byte("hello"), nil); err != nil {
		t.Fatal(err.Error())
	}
	waitSignal(t, started, "background drain")

	cancel()

	err := store.Flush(context.Background())
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestBufferedStoreRestoresOnlyUnwrittenRefRecords(t *testing.T) {
	ctx := context.Background()
	inner := newCountStore(hash.HashType_HashType_BLAKE3)
	inner.recordFailAt = 1
	inner.recordErr = context.DeadlineExceeded
	store := NewBufferedStore(ctx, inner)

	src0, _, err := store.PutBlock(ctx, []byte("src0"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	dst0, _, err := store.PutBlock(ctx, []byte("dst0"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	src1, _, err := store.PutBlock(ctx, []byte("src1"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	dst1, _, err := store.PutBlock(ctx, []byte("dst1"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := store.RecordBlockRefs(ctx, src0, []*BlockRef{dst0}); err != nil {
		t.Fatal(err.Error())
	}
	if err := store.RecordBlockRefs(ctx, src1, []*BlockRef{dst1}); err != nil {
		t.Fatal(err.Error())
	}

	err = store.Flush(ctx)
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if inner.recordCalls != 1 {
		t.Fatalf("expected one successful ref record before failure, got %d", inner.recordCalls)
	}

	inner.recordFailAt = 0
	inner.recordErr = nil
	err = store.Flush(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if inner.recordCalls != 2 {
		t.Fatalf("expected only unwritten ref records to retry, got %d total calls", inner.recordCalls)
	}
}
