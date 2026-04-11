package bucket

import (
	"context"
	"sync"
	"testing"

	hash "github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
)

type bucketRWTestStore struct {
	mtx             sync.Mutex
	putCalls        int
	rmCalls         int
	batchCalls      int
	batchEntries    int
	backgroundCalls int
}

func (s *bucketRWTestStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_BLAKE3
}

func (s *bucketRWTestStore) PutBlock(_ context.Context, _ []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.mtx.Lock()
	s.putCalls++
	s.mtx.Unlock()
	return opts.GetForceBlockRef(), false, nil
}

func (s *bucketRWTestStore) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *bucketRWTestStore) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *bucketRWTestStore) StatBlock(context.Context, *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

func (s *bucketRWTestStore) RmBlock(context.Context, *block.BlockRef) error {
	s.mtx.Lock()
	s.rmCalls++
	s.mtx.Unlock()
	return nil
}

func (s *bucketRWTestStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	s.mtx.Lock()
	s.batchCalls++
	s.batchEntries += len(entries)
	s.mtx.Unlock()
	return nil
}

func (s *bucketRWTestStore) PutBlockBackground(_ context.Context, _ []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.mtx.Lock()
	s.backgroundCalls++
	s.mtx.Unlock()
	return opts.GetForceBlockRef(), false, nil
}

func (s *bucketRWTestStore) getCounts() (int, int, int, int, int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.putCalls, s.rmCalls, s.batchCalls, s.batchEntries, s.backgroundCalls
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
	readStore := &bucketRWTestStore{}
	writeStore := &bucketRWTestStore{}
	readBucket := &bucketRWTestBucket{
		bucketRWTestStore: readStore,
		conf:              &Config{Id: "bucket"},
	}
	writeBucket := &bucketRWTestBucket{
		bucketRWTestStore: writeStore,
		conf:              &Config{Id: "bucket"},
	}
	b := NewBucketRW(readBucket, writeBucket)
	ref := &block.BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{1})}

	batcher, ok := b.(block.BatchPutStore)
	if !ok {
		t.Fatal("expected bucketRW to implement block.BatchPutStore")
	}
	if err := batcher.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: []byte("hello")}}); err != nil {
		t.Fatal(err.Error())
	}
	putCalls, _, batchCalls, _, _ := writeStore.getCounts()
	if batchCalls != 1 || putCalls != 0 {
		t.Fatalf("expected one batch call and no per-entry fallback, got batch=%d put=%d", batchCalls, putCalls)
	}

	bg, ok := b.(block.BackgroundPutStore)
	if !ok {
		t.Fatal("expected bucketRW to implement block.BackgroundPutStore")
	}
	if _, _, err := bg.PutBlockBackground(ctx, []byte("hello"), &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}
	putCalls, _, _, _, backgroundCalls := writeStore.getCounts()
	if backgroundCalls != 1 || putCalls != 0 {
		t.Fatalf("expected one background call and no foreground fallback, got background=%d put=%d", backgroundCalls, putCalls)
	}
}

func TestBucketRWTransactionWriteUsesBatchPutStore(t *testing.T) {
	ctx := context.Background()
	readStore := &bucketRWTestStore{}
	writeStore := &bucketRWTestStore{}
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

	putCalls, _, batchCalls, batchEntries, _ := writeStore.getCounts()
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
