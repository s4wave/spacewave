package kvtx_block

import (
	"bytes"
	"context"
	"iter"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
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

func newSelectorOkraStore() block.StoreOps {
	return block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hash.HashType_HashType_BLAKE3,
		false,
	)
}
