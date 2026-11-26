package provider_spacewave

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/object"
)

// TestVerifiedSOCacheLogWritesDeltas writes a mount's verified cache through
// verifiedSOCacheWriter in the log layout. It checks that an appended op
// stores the same bytes at n and 10n ops, that a write whose commit failed is
// forgotten so no later delta builds on it, that a write after the chain is
// invalidated stores the whole state, and that switching layouts both
// ways loads every state back and leaves the other layout empty.
func TestVerifiedSOCacheLogWritesDeltas(t *testing.T) {
	// Select the log layout for the test and restore the startup selection afterward.
	ctx := t.Context()
	selected := verifiedStateLog
	t.Cleanup(func() { verifiedStateLog = selected })
	verifiedStateLog = true

	// appendCost writes a cache of n ops, then the cache with one more op,
	// and returns the bytes that second write stored.
	appendCost := func(n int) int {
		// Persist the initial state before measuring its next delta.
		store := newVerifiedCacheTestStore()
		w := &verifiedSOCacheWriter{store: store, soID: "so-cost"}
		cache := testVerifiedCache(0, n)
		if err := w.write(ctx, cache, nil); err != nil {
			t.Fatal(err)
		}

		// Count only the bytes written by appending one operation.
		store.setBytes = 0
		if err := w.write(ctx, withVerifiedOps(cache, n, n+1), nil); err != nil {
			t.Fatal(err)
		}
		return store.setBytes
	}
	small, large := appendCost(32), appendCost(320)
	t.Logf("an appended op stores %d bytes at n=32 and %d at n=320", small, large)
	if large > small+8 {
		t.Fatalf("an appended op stored %d bytes at n=32 and %d at n=320", small, large)
	}

	// Mount a writer over a cache of 8 ops.
	store := newVerifiedCacheTestStore()
	acc := &ProviderAccount{objStore: store}
	w := &verifiedSOCacheWriter{store: acc.objStore, soID: "so-1"}
	cache := testVerifiedCache(0, 8)
	if err := w.write(ctx, cache, nil); err != nil {
		t.Fatal(err)
	}

	// A write whose commit fails stores nothing; the next write must still
	// store its op, which a delta over the failed write would leave out.
	store.failCommit = true
	if err := w.write(ctx, withVerifiedOps(cache, 8, 9), nil); err == nil {
		t.Fatal("a failed commit returned no error")
	}
	store.failCommit = false
	cache = withVerifiedOps(cache, 8, 10)
	if err := w.write(ctx, cache, nil); err != nil {
		t.Fatal(err)
	}
	assertVerifiedCache(t, acc, "so-1", cache)

	// Invalidating the chain deletes the logs under the live writer; its next
	// write must store the whole state, which a delta over the deleted logs
	// would leave partial.
	if err := acc.InvalidateVerifiedChain(ctx, "so-1"); err != nil {
		t.Fatal(err)
	}
	cache = withVerifiedOps(cache, 10, 11)
	if err := w.write(ctx, cache, nil); err != nil {
		t.Fatal(err)
	}
	assertVerifiedCache(t, acc, "so-1", cache)

	// A remount in the whole layout moves the states into the record and
	// clears the logs.
	verifiedStateLog = false
	_, written, err := acc.loadVerifiedSOStateCacheFrom(ctx, "so-1")
	if err != nil {
		t.Fatal(err)
	}
	w = &verifiedSOCacheWriter{store: acc.objStore, soID: "so-1", written: written}
	cache = withVerifiedOps(cache, 11, 12)
	if err := w.write(ctx, cache, nil); err != nil {
		t.Fatal(err)
	}
	assertVerifiedCache(t, acc, "so-1", cache)
	if n := countVerifiedKeys(t, store, verifiedSOLogKeyPrefix); n != 0 {
		t.Fatalf("a whole write left %d log keys", n)
	}

	// A remount in the log layout moves them back out of the record.
	verifiedStateLog = true
	_, written, err = acc.loadVerifiedSOStateCacheFrom(ctx, "so-1")
	if err != nil {
		t.Fatal(err)
	}
	w = &verifiedSOCacheWriter{store: acc.objStore, soID: "so-1", written: written}
	cache = withVerifiedOps(cache, 12, 13)
	if err := w.write(ctx, cache, nil); err != nil {
		t.Fatal(err)
	}
	assertVerifiedCache(t, acc, "so-1", cache)
	record := readVerifiedRecord(t, store, "so-1")
	if record.GetPeerState() != nil || record.GetCloudState() != nil {
		t.Fatal("a log write kept a state in the record")
	}

	// Delete removes the record and both logs.
	if err := acc.deleteVerifiedSOStateCache(ctx, "so-1"); err != nil {
		t.Fatal(err)
	}
	if n := countVerifiedKeys(t, store, ""); n != 0 {
		t.Fatalf("delete left %d keys", n)
	}
}

// testVerifiedCache returns a cache whose peer state holds ops from to
// before to, and whose cloud state holds the first of them.
func testVerifiedCache(from, to int) *api.VerifiedSOStateCache {
	peer := &sobject.SOState{Config: &sobject.SharedObjectConfig{ConfigChainSeqno: 1}}
	cache := &api.VerifiedSOStateCache{
		VerifiedConfigChainSeqno: 1,
		PeerState:                peer,
		CloudState:               &sobject.SOState{Config: peer.GetConfig().CloneVT()},
	}
	cache = withVerifiedOps(cache, from, to)
	cache.CloudState.Ops = cache.PeerState.Ops[:1]
	return cache
}

// withVerifiedOps returns a copy of cache whose peer state adds ops from to
// before to, sharing the held op records as SOState.Clone does.
func withVerifiedOps(cache *api.VerifiedSOStateCache, from, to int) *api.VerifiedSOStateCache {
	next := *cache
	next.PeerState = cache.GetPeerState().Clone()
	for i := from; i < to; i++ {
		inner := binary.BigEndian.AppendUint64([]byte("op"), uint64(i))
		next.PeerState.Ops = append(next.PeerState.Ops, &sobject.SOOperation{Inner: inner})
	}
	slices.SortFunc(next.PeerState.Ops, func(a, b *sobject.SOOperation) int {
		return bytes.Compare(a.Hash(), b.Hash())
	})
	return &next
}

// assertVerifiedCache checks that soID's cache loads back equal to want.
func assertVerifiedCache(t *testing.T, acc *ProviderAccount, soID string, want *api.VerifiedSOStateCache) {
	t.Helper()
	loaded, err := acc.loadVerifiedSOStateCache(t.Context(), soID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.EqualVT(want) {
		t.Fatalf("loaded cache differs:\n%v\n%v", loaded, want)
	}
}

// readVerifiedRecord decodes soID's stored record as it is, without its logs.
func readVerifiedRecord(t *testing.T, store kvtx.Store, soID string) *api.VerifiedSOStateCache {
	t.Helper()
	tx, err := store.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	data, _, err := tx.Get(t.Context(), verifiedSOStateCacheKey(soID))
	if err != nil {
		t.Fatal(err)
	}
	record := &api.VerifiedSOStateCache{}
	if err := record.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	return record
}

// countVerifiedKeys counts the keys of store under prefix.
func countVerifiedKeys(t *testing.T, store kvtx.Store, prefix string) int {
	t.Helper()
	tx, err := store.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var n int
	err = tx.ScanPrefixKeys(t.Context(), []byte(prefix), func([]byte) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// verifiedCacheTestStore counts the bytes its writes set and can fail a
// commit, discarding the transaction.
type verifiedCacheTestStore struct {
	object.ObjectStore
	setBytes   int
	failCommit bool
}

// newVerifiedCacheTestStore returns an empty in-memory test store.
func newVerifiedCacheTestStore() *verifiedCacheTestStore {
	return &verifiedCacheTestStore{ObjectStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
}

// NewTransaction wraps a write transaction to count and fail it.
func (s *verifiedCacheTestStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.ObjectStore.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &verifiedCacheTestTx{Tx: tx, s: s}, nil
}

// verifiedCacheTestTx is a write transaction of verifiedCacheTestStore.
type verifiedCacheTestTx struct {
	kvtx.Tx
	s *verifiedCacheTestStore
}

// Set counts the key and value bytes.
func (tx *verifiedCacheTestTx) Set(ctx context.Context, key, value []byte) error {
	tx.s.setBytes += len(key) + len(value)
	return tx.Tx.Set(ctx, key, value)
}

// Commit discards the transaction when the store fails commits.
func (tx *verifiedCacheTestTx) Commit(ctx context.Context) error {
	if tx.s.failCommit {
		tx.Discard()
		return errors.New("test commit failure")
	}
	return tx.Tx.Commit(ctx)
}
