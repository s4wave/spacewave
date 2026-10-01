//go:build !js && !wasip1

package volume_controller

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// TestBucketReadOperationUsesOneSnapshot requires the GC wrapper and volume to share a reader.
func TestBucketReadOperationUsesOneSnapshot(t *testing.T) {
	t.Run("plain", func(t *testing.T) { testBucketReadOperationUsesOneSnapshot(t, false) })
	t.Run("gc", func(t *testing.T) { testBucketReadOperationUsesOneSnapshot(t, true) })
}

// testBucketReadOperationUsesOneSnapshot checks either native bucket wrapper chain.
func testBucketReadOperationUsesOneSnapshot(t *testing.T, withGC bool) {
	// Assemble the native Bolt-backed volume with the ordinary GC wrapper.
	t.Helper()
	store, err := store_kvtx_bolt.Open(filepath.Join(t.TempDir(), "bucket.db"), 0o600, nil, []byte("volume"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.GetDB().Close() })

	// Construct the volume over the Bolt store.
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
	if count := store.GetDB().Stats().OpenTxN; count != 1 {
		t.Fatalf("bucket scope opened %d database snapshots, want one", count)
	}
	nested, releaseNested, err := scoped.BeginReadOperation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseNested)
	if count := store.GetDB().Stats().OpenTxN; count != 1 {
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
	if count := store.GetDB().Stats().OpenTxN; count != 0 {
		t.Fatalf("released bucket scope retained %d database snapshots", count)
	}
}
