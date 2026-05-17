package bucket

import (
	"context"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hash "github.com/s4wave/spacewave/net/hash"
)

type bucketRWTestStore struct {
	block.StoreOps

	mtx              sync.Mutex
	putCalls         int
	rmCalls          int
	batchCalls       int
	batchEntries     int
	backgroundCalls  int
	existsBatchCalls int
}

func newBucketRWTestStore() *bucketRWTestStore {
	return &bucketRWTestStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			hash.HashType_HashType_BLAKE3,
			false,
		),
	}
}

func (s *bucketRWTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.mtx.Lock()
	s.putCalls++
	s.mtx.Unlock()
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *bucketRWTestStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	s.mtx.Lock()
	s.rmCalls++
	s.mtx.Unlock()
	return s.StoreOps.RmBlock(ctx, ref)
}

func (s *bucketRWTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.mtx.Lock()
	s.batchCalls++
	s.batchEntries += len(entries)
	s.mtx.Unlock()
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *bucketRWTestStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.mtx.Lock()
	s.backgroundCalls++
	s.mtx.Unlock()
	return s.StoreOps.PutBlockBackground(ctx, data, opts)
}

func (s *bucketRWTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.mtx.Lock()
	s.existsBatchCalls++
	s.mtx.Unlock()
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

func (s *bucketRWTestStore) getCounts() (int, int, int, int, int, int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.putCalls, s.rmCalls, s.batchCalls, s.batchEntries, s.backgroundCalls, s.existsBatchCalls
}

type bucketRWTestBucket struct {
	*bucketRWTestStore
	conf *Config
}

func (b *bucketRWTestBucket) GetBucketConfig() *Config {
	return b.conf
}

func TestBucketRWForwardsBlockStoreExtensions(t *testing.T) {
	ctx := context.Background()
	readStore := newBucketRWTestStore()
	writeStore := newBucketRWTestStore()
	readBucket := &bucketRWTestBucket{
		bucketRWTestStore: readStore,
		conf:              &Config{Id: "bucket"},
	}
	writeBucket := &bucketRWTestBucket{
		bucketRWTestStore: writeStore,
		conf:              &Config{Id: "bucket"},
	}
	b := NewBucketRW(readBucket, writeBucket)
	ref, err := block.BuildBlockRef([]byte("hello"), &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3})
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := b.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: []byte("hello")}}); err != nil {
		t.Fatal(err.Error())
	}
	putCalls, _, batchCalls, _, _, _ := writeStore.getCounts()
	if batchCalls != 1 || putCalls != 0 {
		t.Fatalf("expected one batch call and no per-entry fallback, got batch=%d put=%d", batchCalls, putCalls)
	}

	if _, _, err := b.PutBlockBackground(ctx, []byte("hello"), &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}
	putCalls, _, _, _, backgroundCalls, _ := writeStore.getCounts()
	if backgroundCalls != 1 || putCalls != 0 {
		t.Fatalf("expected one background call and no foreground fallback, got background=%d put=%d", backgroundCalls, putCalls)
	}

	if _, err := b.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatal(err.Error())
	}
	_, _, _, _, _, existsBatchCalls := readStore.getCounts()
	if existsBatchCalls != 1 {
		t.Fatalf("expected one exists batch call and no fallback, got %d", existsBatchCalls)
	}
}

func TestBucketRWTransactionWriteUsesBatchPut(t *testing.T) {
	ctx := context.Background()
	readStore := newBucketRWTestStore()
	writeStore := newBucketRWTestStore()
	readBucket := &bucketRWTestBucket{
		bucketRWTestStore: readStore,
		conf:              &Config{Id: "bucket"},
	}
	writeBucket := &bucketRWTestBucket{
		bucketRWTestStore: writeStore,
		conf:              &Config{Id: "bucket"},
	}

	tx, root := block.NewTransaction(NewBucketRW(readBucket, writeBucket), nil, nil, nil)
	root.SetBlock(&block_mock.Root{}, true)
	sub := root.FollowSubBlock(1)
	ref := sub.FollowRef(1, nil)
	ref.SetBlock(block_mock.NewExample("hello world"), true)

	if _, _, err := tx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}

	putCalls, _, batchCalls, batchEntries, _, _ := writeStore.getCounts()
	if batchCalls == 0 {
		t.Fatal("expected transaction write to use PutBlockBatch on the write bucket")
	}
	if putCalls != 0 {
		t.Fatalf("expected no per-entry PutBlock fallback, got %d calls", putCalls)
	}
	if batchEntries != 2 {
		t.Fatalf("expected exactly 2 batch entries for root + child block, got %d", batchEntries)
	}
}
