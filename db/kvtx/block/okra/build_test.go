package kvtx_block_okra

import (
	"bytes"
	"context"
	"iter"
	"slices"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

func TestBuildTreeReadOnlyLookup(t *testing.T) {
	// Create the context, store and sorted fixture for the Okra builder test.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 64)

	// Build and publish the sorted fixture tree.
	tx, _, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the published root as a read-only Okra transaction.
	_, readCursor := block.NewTransaction(store, nil, rootRef, nil)
	okraTx, err := NewTx(ctx, readCursor, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer okraTx.Discard()

	// Require the published size to match the fixture key count.
	size, err := okraTx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != uint64(len(fixture.keys)) {
		t.Fatalf("size = %d, want %d", size, len(fixture.keys))
	}

	// Read a fixture value through the key lookup API.
	key := fixture.keys[17]
	value, found, err := okraTx.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}

	// Require the fixture key and its expected value bytes.
	if !found {
		t.Fatalf("key %q not found", key)
	}
	if !bytes.Equal(value, fixture.values[17]) {
		t.Fatalf("value = %q, want %q", value, fixture.values[17])
	}

	// Require the existing fixture key to report present.
	exists, err := okraTx.Exists(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("exists(%q) = false", key)
	}

	// Require a missing key to report absent.
	exists, err = okraTx.Exists(ctx, []byte("key-999999"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("missing key reported present")
	}

	// Fetch the fixture value through its block cursor.
	valueCursor, err := okraTx.GetCursorAtKey(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data, ok, err := valueCursor.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Require the fetched cursor bytes to match the fixture value.
	if !ok || !bytes.Equal(data, fixture.values[17]) {
		t.Fatalf("cursor value = %q, %v, want %q, true", data, ok, fixture.values[17])
	}
}

func TestBuildTreeStableRoot(t *testing.T) {
	// Create the context, store and sorted fixture for the Okra builder test.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 128)

	// Publish the fixture twice to compare their roots.
	firstRef, firstRoot := writeOkraFixture(t, ctx, store, fixture)
	secondRef, secondRoot := writeOkraFixture(t, ctx, store, fixture)

	// Require stable root references and hashes with the configured tree constants.
	if !firstRef.EqualsRef(secondRef) {
		t.Fatalf("root ref changed: %s != %s", firstRef.MarshalLog(), secondRef.MarshalLog())
	}
	if !bytes.Equal(firstRoot.GetRootHash(), secondRoot.GetRootHash()) {
		t.Fatalf("root hash changed: %x != %x", firstRoot.GetRootHash(), secondRoot.GetRootHash())
	}
	if firstRoot.GetHashSize() != HashSize || firstRoot.GetFanoutDegree() != FanoutDegree {
		t.Fatalf(
			"root constants = K=%d Q=%d, want K=%d Q=%d",
			firstRoot.GetHashSize(),
			firstRoot.GetFanoutDegree(),
			HashSize,
			FanoutDegree,
		)
	}
}

func TestIncrementalRootMatchesBuilder(t *testing.T) {
	// Create the context, store and sorted fixture for the Okra builder test.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 512)

	// Prepare the base tree and exercise each mutation against the builder.
	baseRef, _ := writeOkraFixture(t, ctx, store, fixture)
	pageStartKey := firstNonAnchorLeafStartKey(t, ctx, store, baseRef)
	for _, tc := range []struct {
		name   string
		apply  func([]BuildEntry) []BuildEntry
		mutate func(*Tx, *block.Cursor)
	}{
		{
			name: "update-existing",
			apply: func(entries []BuildEntry) []BuildEntry {
				updatedRef := putOkraTestRef(t, ctx, store, "updated-value")
				return setBuildEntry(entries, fixture.keys[257], updatedRef)
			},
			mutate: func(tx *Tx, rootCursor *block.Cursor) {
				updatedRef := putOkraTestRef(t, ctx, store, "updated-value")
				setOkraRefAtKey(t, ctx, rootCursor, tx, fixture.keys[257], updatedRef)
			},
		},
		{
			name: "insert-page-split",
			apply: func(entries []BuildEntry) []BuildEntry {
				for i := range 80 {
					key := []byte("key-0257-extra-" + strconv.FormatInt(int64(i), 10))
					entries = setBuildEntry(entries, key, putOkraTestRef(t, ctx, store, "inserted-"+strconv.FormatInt(int64(i), 10)))
				}
				return entries
			},
			mutate: func(tx *Tx, rootCursor *block.Cursor) {
				for i := range 80 {
					key := []byte("key-0257-extra-" + strconv.FormatInt(int64(i), 10))
					setOkraRefAtKey(t, ctx, rootCursor, tx, key, putOkraTestRef(t, ctx, store, "inserted-"+strconv.FormatInt(int64(i), 10)))
				}
			},
		},
		{
			name: "delete-page-start-key",
			apply: func(entries []BuildEntry) []BuildEntry {
				return deleteBuildEntry(entries, pageStartKey)
			},
			mutate: func(tx *Tx, _ *block.Cursor) {
				if err := tx.Delete(ctx, pageStartKey); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "delete-merge-root-shrink",
			apply: func(entries []BuildEntry) []BuildEntry {
				keep := fixture.keys[0]
				for _, key := range fixture.keys[1:] {
					entries = deleteBuildEntry(entries, key)
				}
				return setBuildEntry(entries, keep, fixture.refs[0])
			},
			mutate: func(tx *Tx, _ *block.Cursor) {
				for _, key := range fixture.keys[1:] {
					if err := tx.Delete(ctx, key); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "delete-reinsert",
			apply: func(entries []BuildEntry) []BuildEntry {
				reinsertedRef := putOkraTestRef(t, ctx, store, "reinserted-value")
				entries = deleteBuildEntry(entries, fixture.keys[401])
				return setBuildEntry(entries, fixture.keys[401], reinsertedRef)
			},
			mutate: func(tx *Tx, rootCursor *block.Cursor) {
				if err := tx.Delete(ctx, fixture.keys[401]); err != nil {
					t.Fatal(err)
				}
				reinsertedRef := putOkraTestRef(t, ctx, store, "reinserted-value")
				setOkraRefAtKey(t, ctx, rootCursor, tx, fixture.keys[401], reinsertedRef)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Apply the expected entry changes to a copy of the fixture.
			entries := tc.apply(cloneFixtureEntries(fixture))

			// Require the incremental mutation to match an independent rebuild.
			assertIncrementalRootMatchesBuilder(t, ctx, store, baseRef, entries, tc.mutate)
		})
	}
}

func cloneFixtureEntries(fixture *okraFixture) []BuildEntry {
	entries := make([]BuildEntry, len(fixture.keys))
	for idx := range fixture.keys {
		entries[idx] = BuildEntry{
			Key:      bytes.Clone(fixture.keys[idx]),
			ValueRef: fixture.refs[idx].Clone(),
		}
	}
	return entries
}

func putOkraTestRef(t *testing.T, ctx context.Context, store block.StoreOps, value string) *block.BlockRef {
	t.Helper()
	ref, _, err := store.PutBlock(ctx, []byte(value), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func setBuildEntry(entries []BuildEntry, key []byte, ref *block.BlockRef) []BuildEntry {
	for idx := range entries {
		if bytes.Equal(entries[idx].Key, key) {
			entries[idx].ValueRef = ref.Clone()
			return entries
		}
	}
	entries = append(entries, BuildEntry{
		Key:      bytes.Clone(key),
		ValueRef: ref.Clone(),
	})
	slices.SortFunc(entries, func(a, b BuildEntry) int {
		return bytes.Compare(a.Key, b.Key)
	})
	return entries
}

func deleteBuildEntry(entries []BuildEntry, key []byte) []BuildEntry {
	for idx := range entries {
		if bytes.Equal(entries[idx].Key, key) {
			return slices.Delete(entries, idx, idx+1)
		}
	}
	return entries
}

func setOkraRefAtKey(
	t *testing.T,
	ctx context.Context,
	rootCursor *block.Cursor,
	tx *Tx,
	key []byte,
	ref *block.BlockRef,
) {
	// Attach the supplied value reference through a detached cursor.
	t.Helper()
	valueCursor := rootCursor.Detach(false)
	valueCursor.ClearAllRefs()
	valueCursor.SetRefAtCursor(ref, true)
	if err := tx.SetCursorAtKey(ctx, key, valueCursor, false); err != nil {
		t.Fatal(err)
	}
}

func assertIncrementalRootMatchesBuilder(
	t *testing.T,
	ctx context.Context,
	store block.StoreOps,
	baseRef *block.BlockRef,
	entries []BuildEntry,
	mutate func(*Tx, *block.Cursor),
) {
	// Build the expected root from the changed entry sequence.
	t.Helper()
	builderRef, builderRoot := writeOkraBuildEntries(t, ctx, store, entries)

	// Open the base tree for the incremental mutation.
	btx, rootCursor := block.NewTransaction(store, nil, baseRef, nil)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Apply and publish the incremental tree mutation.
	mutate(tx, rootCursor)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	incrementalRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the incrementally published tree for comparison.
	readTx := openOkraRoot(t, ctx, store, incrementalRef, false)
	defer readTx.Discard()

	// Require the incremental root reference and hash to match the builder.
	if !builderRef.EqualsRef(incrementalRef) {
		t.Fatalf(
			"incremental root ref = %s, builder root ref = %s, incremental height/size/hash = %d/%d/%x, builder height/size/hash = %d/%d/%x",
			incrementalRef.MarshalLog(),
			builderRef.MarshalLog(),
			readTx.root.GetHeight(),
			readTx.root.GetSize(),
			readTx.root.GetRootHash(),
			builderRoot.GetHeight(),
			builderRoot.GetSize(),
			builderRoot.GetRootHash(),
		)
	}
	if !bytes.Equal(readTx.root.GetRootHash(), builderRoot.GetRootHash()) {
		t.Fatalf("incremental root hash = %x, builder root hash = %x", readTx.root.GetRootHash(), builderRoot.GetRootHash())
	}
}

func firstNonAnchorLeafStartKey(
	t *testing.T,
	ctx context.Context,
	store block.StoreOps,
	rootRef *block.BlockRef,
) []byte {
	// Open the fixture root and load its top page.
	t.Helper()
	tx := openOkraRoot(t, ctx, store, rootRef, false)
	defer tx.Discard()
	page, cursor, err := tx.getRootPage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Locate the first leaf boundary that follows the anchor page.
	key, ok := firstNonAnchorLeafStartKeyInPage(t, ctx, page, cursor)
	if !ok {
		t.Fatal("missing non-anchor leaf page start key")
	}
	return key
}

func firstNonAnchorLeafStartKeyInPage(
	t *testing.T,
	ctx context.Context,
	page *Page,
	cursor *block.Cursor,
) ([]byte, bool) {
	// Return the first key of a non-anchor leaf page.
	t.Helper()
	if page.GetLevel() == 0 {
		if page.GetStartsAtAnchor() {
			return nil, false
		}
		entries := page.GetEntries()
		if len(entries) == 0 {
			t.Fatal("leaf page has no entries")
		}
		return bytes.Clone(entries[0].GetKey()), true
	}

	// Search child pages for the first non-anchor leaf boundary.
	for idx := range page.GetEntries() {
		childCursor := page.FollowChild(cursor, idx)
		child, err := loadPage(ctx, childCursor)
		if err != nil {
			t.Fatal(err)
		}
		if key, ok := firstNonAnchorLeafStartKeyInPage(t, ctx, child, childCursor); ok {
			return key, true
		}
	}
	return nil, false
}

func writeOkraBuildEntries(
	t *testing.T,
	ctx context.Context,
	store block.StoreOps,
	entries []BuildEntry,
) (*block.BlockRef, *Root) {
	// Build the requested entry sequence with staged page writes.
	t.Helper()
	builderTx, _, err := BuildTreeWithEntries(ctx, store, nil, nil, func(yield func(BuildEntry) bool) {
		for _, ent := range entries {
			if !yield(ent) {
				return
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Publish the built root to the block store.
	rootRef, _, err := builderTx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Reopen the published root and return a copy of its metadata.
	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()
	return rootRef, readTx.root.CloneVT()
}

func TestBuildTreeRootPageMetadata(t *testing.T) {
	// Create the context, store and sorted fixture for the Okra builder test.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 96)
	_, root := writeOkraFixture(t, ctx, store, fixture)

	// Load the built root page for metadata comparison.
	_, readCursor := block.NewTransaction(store, nil, root.GetRootPageRef(), nil)
	page, err := loadPage(ctx, readCursor)
	if err != nil {
		t.Fatal(err)
	}

	// Require the root page level, hash, entries and size to agree with the root.
	if page.GetLevel() != root.GetHeight() {
		t.Fatalf("root page level = %d, want %d", page.GetLevel(), root.GetHeight())
	}
	if len(page.GetPageHash()) != HashSize {
		t.Fatalf("root page hash length = %d, want %d", len(page.GetPageHash()), HashSize)
	}
	if len(page.GetEntries()) == 0 {
		t.Fatal("root page has no entries")
	}
	if page.GetSize() != root.GetSize() {
		t.Fatalf("root page size = %d, want %d", page.GetSize(), root.GetSize())
	}
}

func TestBuildTreeDepthIsLowerThanIAVL(t *testing.T) {
	// Create the context, store and sorted fixture for the Okra builder test.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 1024)
	_, okraRoot := writeOkraFixture(t, ctx, store, fixture)

	// Build and publish the same fixture as an IAVL tree.
	iavlTx, _, err := iavl.BuildTree(store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	iavlRef, _, err := iavlTx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the published IAVL tree for a height comparison.
	_, iavlReadCursor := block.NewTransaction(store, nil, iavlRef, nil)
	iavlReadTx, err := iavl.NewTx(ctx, iavlReadCursor, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer iavlReadTx.Discard()

	// Require the packed Okra tree to have lower height than IAVL.
	if okraRoot.GetHeight() >= iavlReadTx.Height() {
		t.Fatalf("Okra height = %d, IAVL height = %d", okraRoot.GetHeight(), iavlReadTx.Height())
	}
}

func TestBuildTreeRejectsUnsortedEntries(t *testing.T) {
	// Feed the Okra builder an entry sequence with descending keys.
	ctx := context.Background()
	store := newOkraTestStore()
	_, _, err := BuildTreeWithEntries(ctx, store, nil, nil, func(yield func(BuildEntry) bool) {
		yield(BuildEntry{Key: []byte("b")})
		yield(BuildEntry{Key: []byte("a")})
	})

	// Require the builder to reject the unsorted entry sequence.
	if err != ErrUnsortedEntries {
		t.Fatalf("err = %v, want %v", err, ErrUnsortedEntries)
	}
}

type okraFixture struct {
	keys   [][]byte
	values [][]byte
	refs   []*block.BlockRef
}

func newOkraFixture(t *testing.T, ctx context.Context, store block.StoreOps, size int) *okraFixture {
	t.Helper()
	out := &okraFixture{
		keys:   make([][]byte, size),
		values: make([][]byte, size),
		refs:   make([]*block.BlockRef, size),
	}
	for i := range size {
		key := []byte("key-" + strconv.FormatInt(int64(i), 10))
		value := []byte("value-" + strconv.FormatInt(int64(i), 10))
		if i < 10 {
			key = []byte("key-000" + strconv.FormatInt(int64(i), 10))
		} else if i < 100 {
			key = []byte("key-00" + strconv.FormatInt(int64(i), 10))
		} else if i < 1000 {
			key = []byte("key-0" + strconv.FormatInt(int64(i), 10))
		}
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

func (o *okraFixture) seq() iter.Seq2[[]byte, *block.BlockRef] {
	return func(yield func([]byte, *block.BlockRef) bool) {
		for idx := range o.keys {
			if !yield(o.keys[idx], o.refs[idx]) {
				return
			}
		}
	}
}

func writeOkraFixture(
	t *testing.T,
	ctx context.Context,
	store block.StoreOps,
	fixture *okraFixture,
) (*block.BlockRef, *Root) {
	// Build the sorted fixture as a staged Okra tree.
	t.Helper()
	tx, rootCursor, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}

	// Publish the fixture root to the block store.
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Reopen the published fixture root for metadata readback.
	_, readCursor := block.NewTransaction(store, nil, rootRef, nil)
	okraTx, err := NewTx(ctx, readCursor, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer okraTx.Discard()

	// Require the builder to return the root cursor with its metadata.
	if rootCursor == nil {
		t.Fatal("missing root cursor")
	}
	return rootRef, okraTx.root.CloneVT()
}

func newOkraTestStore() block.StoreOps {
	return block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hash.HashType_HashType_BLAKE3,
		false,
	)
}
