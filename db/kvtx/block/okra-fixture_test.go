package kvtx_block

import (
	"bytes"
	"context"
	"iter"
	"strconv"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	"github.com/s4wave/spacewave/net/hash"
)

func TestOkraFixtureThroughSelector(t *testing.T) {
	ctx := context.Background()
	store := newSelectorOkraStore()
	fixture := newSelectorOkraFixture(t, ctx, store, 48)

	treeTx, okraRootCursor, err := okra.BuildTree(store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	kvsCursor := okraRootCursor.Detach(false)
	kvsCursor.ClearAllRefs()
	kvsCursor.SetBlock(NewKeyValueStore(KVImplType_KV_IMPL_TYPE_OKRA), true)
	if err := okraRootCursor.SetAsSubBlock(3, kvsCursor); err != nil {
		t.Fatal(err)
	}
	if err := treeTx.SetRoot(kvsCursor); err != nil {
		t.Fatal(err)
	}
	kvsRef, _, err := treeTx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	_, readCursor := block.NewTransaction(store, nil, kvsRef, nil)
	ktx, err := BuildKvTransaction(ctx, readCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ktx.Discard()

	key := fixture.keys[23]
	value, found, err := ktx.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(value, fixture.values[23]) {
		t.Fatalf("Get(%q) = %q, %v, want %q, true", key, value, found, fixture.values[23])
	}

	exists, err := ktx.Exists(ctx, []byte("key-9999"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("missing key reported present")
	}
}

func TestOkraMutationThroughSelector(t *testing.T) {
	ctx := context.Background()
	store := newSelectorOkraStore()

	btx, kvsCursor := block.NewTransaction(store, nil, nil, nil)
	kvsCursor.SetBlock(NewKeyValueStore(KVImplType_KV_IMPL_TYPE_OKRA), true)
	ktx, err := BuildKvTransaction(ctx, kvsCursor, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := ktx.Set(ctx, []byte("b"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := ktx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := ktx.Delete(ctx, []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := ktx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	_, readCursor := block.NewTransaction(store, nil, rootRef, nil)
	readTx, err := BuildKvTransaction(ctx, readCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	value, found, err := readTx.Get(ctx, []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(value, []byte("1")) {
		t.Fatalf("Get(a) = %q, %v, want 1, true", value, found)
	}
	exists, err := readTx.Exists(ctx, []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("deleted key exists after selector commit")
	}
}

type selectorOkraFixture struct {
	keys   [][]byte
	values [][]byte
	refs   []*block.BlockRef
}

func newSelectorOkraFixture(
	t *testing.T,
	ctx context.Context,
	store block.StoreOps,
	size int,
) *selectorOkraFixture {
	t.Helper()
	out := &selectorOkraFixture{
		keys:   make([][]byte, size),
		values: make([][]byte, size),
		refs:   make([]*block.BlockRef, size),
	}
	for i := range size {
		key := []byte("key-" + strconv.FormatInt(int64(i+1000), 10))
		value := []byte("selector-value-" + strconv.FormatInt(int64(i), 10))
		ref, _, err := store.PutBlock(ctx, value, nil)
		if err != nil {
			t.Fatal(err)
		}
		out.keys[i] = key
		out.values[i] = value
		out.refs[i] = ref
	}
	return out
}

func (o *selectorOkraFixture) seq() iter.Seq2[[]byte, *block.BlockRef] {
	return func(yield func([]byte, *block.BlockRef) bool) {
		for idx := range o.keys {
			if !yield(o.keys[idx], o.refs[idx]) {
				return
			}
		}
	}
}

type selectorOkraStore struct {
	block.NopStoreOps

	mtx    sync.Mutex
	blocks map[string][]byte
}

func newSelectorOkraStore() *selectorOkraStore {
	return &selectorOkraStore{blocks: make(map[string][]byte)}
}

func (s *selectorOkraStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_BLAKE3
}

func (s *selectorOkraStore) PutBlock(
	_ context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	ref := opts.GetForceBlockRef()
	var err error
	if ref.GetEmpty() {
		ref, err = block.BuildBlockRef(data, opts)
		if err != nil {
			return nil, false, err
		}
	} else {
		ref = ref.Clone()
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	key := ref.MarshalString()
	_, exists := s.blocks[key]
	s.blocks[key] = bytes.Clone(data)
	return ref, exists, nil
}

func (s *selectorOkraStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, ent := range entries {
		if ent.Tombstone {
			if ent.Ref != nil {
				s.mtx.Lock()
				delete(s.blocks, ent.Ref.MarshalString())
				s.mtx.Unlock()
			}
			continue
		}
		opts := &block.PutOpts{Refs: ent.Refs}
		if ent.Ref != nil {
			opts.ForceBlockRef = ent.Ref.Clone()
		}
		if _, _, err := s.PutBlock(ctx, ent.Data, opts); err != nil {
			return err
		}
	}
	return nil
}

func (s *selectorOkraStore) PutBlockBackground(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	return s.PutBlock(ctx, data, opts)
}

func (s *selectorOkraStore) GetBlock(
	_ context.Context,
	ref *block.BlockRef,
) ([]byte, bool, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	data, ok := s.blocks[ref.MarshalString()]
	if !ok {
		return nil, false, nil
	}
	return bytes.Clone(data), true, nil
}

func (s *selectorOkraStore) GetBlockExists(
	_ context.Context,
	ref *block.BlockRef,
) (bool, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	_, ok := s.blocks[ref.MarshalString()]
	return ok, nil
}
