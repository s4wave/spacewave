//go:build !js && !wasip1

package volume_controller

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// snapshotStore counts the read transactions open on its database.
type snapshotStore struct {
	*s4db.DB
	// open is the number of read transactions not yet discarded.
	open atomic.Int64
}

// NewTransaction opens a transaction, counting a read until its Discard.
func (s *snapshotStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.DB.NewTransaction(ctx, write)
	if err != nil || write {
		return tx, err
	}
	s.open.Add(1)
	return &snapshotTx{Tx: tx, store: s}, nil
}

// snapshotTx is a counted read transaction.
type snapshotTx struct {
	kvtx.Tx
	store *snapshotStore
	once  sync.Once
}

// Discard discards the transaction and uncounts it once.
func (t *snapshotTx) Discard() {
	t.Tx.Discard()
	t.once.Do(func() { t.store.open.Add(-1) })
}

// TestBucketReadOperationUsesOneSnapshot requires the GC wrapper and volume to share a reader.
func TestBucketReadOperationUsesOneSnapshot(t *testing.T) {
	t.Run("plain", func(t *testing.T) { testBucketReadOperationUsesOneSnapshot(t, false) })
	t.Run("gc", func(t *testing.T) { testBucketReadOperationUsesOneSnapshot(t, true) })
}

// testBucketReadOperationUsesOneSnapshot checks either native bucket wrapper chain.
func testBucketReadOperationUsesOneSnapshot(t *testing.T, withGC bool) {
	// Assemble the native s4db-backed volume with the ordinary GC wrapper.
	t.Helper()
	db, err := s4db.Open(filepath.Join(t.TempDir(), "bucket.s4wave"), s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := &snapshotStore{DB: db}

	// Construct the volume over the s4db store.
	vol, err := common_kvtx.NewVolume(t.Context(), "test-volume", store_kvkey.NewDefaultKVKey(), store, nil, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	// Build the bucket handle with optional GC wrapping.
	handle := &bucketHandle{v: vol, bucketConf: &bucket.Config{Id: "test"}}
	if withGC {
		handle.gcOps = block_gc.NewGCStoreOps(vol, stubCollectorGraph{})
	}

	// Store one block to read back through the scope.
	ref, _, err := vol.PutBlock(t.Context(), []byte("snapshot contents"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// One bucket scope opens one database transaction, including nested read scopes.
	scoped, release, err := handle.BeginReadOperation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if count := store.open.Load(); count != 1 {
		t.Fatalf("bucket scope opened %d database snapshots, want one", count)
	}
	nested, releaseNested, err := scoped.BeginReadOperation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseNested)
	if count := store.open.Load(); count != 1 {
		t.Fatalf("nested bucket scope opened %d database snapshots, want one", count)
	}

	// Read through the nested wrapper before releasing the single underlying snapshot.
	data, found, err := nested.GetBlock(t.Context(), ref)
	if err != nil || !found || string(data) != "snapshot contents" {
		t.Fatalf("nested read returned %q, found=%v, err=%v", data, found, err)
	}
	if _, _, err := nested.PutBlock(t.Context(), []byte("forbidden write"), &block.PutOpts{}); err == nil {
		t.Fatal("read scope accepted a write")
	}
	releaseNested()
	release()
	if count := store.open.Load(); count != 0 {
		t.Fatalf("released bucket scope retained %d database snapshots", count)
	}
}
