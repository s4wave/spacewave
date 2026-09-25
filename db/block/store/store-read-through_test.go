package block_store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/net/hash"
)

func newReadThroughTestBlockStore() *block_store_kvtx.KVTxBlock {
	return block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
		hash.HashType_HashType_BLAKE3,
		false,
	)
}

// refsTestStore records the refs of each put and serves them with the bytes.
type refsTestStore struct {
	block.StoreOps

	mu   sync.Mutex
	refs map[string][]*block.BlockRef
}

func newRefsTestStore() *refsTestStore {
	return &refsTestStore{
		StoreOps: newReadThroughTestBlockStore(),
		refs:     make(map[string][]*block.BlockRef),
	}
}

func (s *refsTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	s.refs[ref.MarshalString()] = opts.GetRefs()
	s.mu.Unlock()
	return ref, existed, nil
}

func (s *refsTestStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

func (s *refsTestStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data, Refs: s.getRefs(ref), RefsKnown: true}, nil
}

func (s *refsTestStore) getRefs(ref *block.BlockRef) []*block.BlockRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refs[ref.MarshalString()]
}

func TestStoreReadThroughWritebackUsesWritablePrimary(t *testing.T) {
	ctx := context.Background()
	primary := newRefsTestStore()
	lower := newRefsTestStore()
	child, _, err := lower.PutBlock(ctx, []byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("read-through writeback")
	ref, _, err := lower.PutBlock(ctx, data, &block.PutOpts{Refs: []*block.BlockRef{child}})
	if err != nil {
		t.Fatal(err)
	}

	if _, found, err := primary.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("primary before read found=%v err=%v, want absent", found, err)
	}

	var lowerSource block.StoreOps = lower
	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		func() block.StoreOps { return lowerSource },
		true,
	)
	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := scoped.GetBlock(ctx, ref)
	release()
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("scoped lower read = %q/%v/%v", got, found, err)
	}

	lowerSource = nil
	got, found, err = primary.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("primary writeback = %q/%v/%v", got, found, err)
	}
	refs := primary.getRefs(ref)
	if len(refs) != 1 || !refs[0].EqualsRef(child) {
		t.Fatalf("primary writeback refs = %v, want [%v]", refs, child)
	}
}

func TestStoreReadThroughByteOnlySourceSkipsFill(t *testing.T) {
	ctx := context.Background()
	primary := newRefsTestStore()
	lower := newReadThroughTestBlockStore()
	data := []byte("byte-only source")
	ref, _, err := lower.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		func() block.StoreOps { return lower },
		true,
	)
	got, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("lower read = %q/%v/%v", got, found, err)
	}
	stored, err := store.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || stored.RefsKnown {
		t.Fatalf("GetStoredBlock = %v/%v, want bytes without refs", stored, err)
	}
	if _, found, err := primary.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("primary after byte-only read found=%v err=%v, want absent", found, err)
	}
}
