//go:build js

package volume_opfs

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/opfs"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/sirupsen/logrus"
)

func TestOpfsVolumeIntegration(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	ctx := context.Background()
	vol, err := NewOpfs(ctx, logrus.NewEntry(logrus.New()), &Config{
		RootPath:    "test-volume-js-opfs",
		StoreConfig: &store_kvtx.Config{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := vol.Delete(); err != nil {
			t.Error(err)
		}
	}()

	if err := store_test.TestAll(ctx, vol); err != nil {
		t.Fatal(err)
	}

	ref, _, err := vol.PutBlock(ctx, []byte("stats-block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := vol.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(data) != "stats-block" {
		t.Fatalf("GetBlock: found=%v data=%q", found, data)
	}

	stats, err := vol.GetStorageStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.GetBlockCount() != 1 {
		t.Fatalf("BlockCount: got %d want 1", stats.GetBlockCount())
	}
	if stats.GetTotalBytes() != uint64(len(data)) {
		t.Fatalf("TotalBytes: got %d want %d", stats.GetTotalBytes(), len(data))
	}
}

func TestOpfsVolumeMaintenanceCompactsBlockshard(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	ctx := context.Background()
	rootPath := "test-volume-js-opfs-maintenance"
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	_ = opfs.DeleteEntry(root, rootPath, true)

	vol, err := NewOpfs(ctx, logrus.NewEntry(logrus.New()), &Config{
		RootPath:               rootPath,
		StoreConfig:            &store_kvtx.Config{},
		BlockShardCount:        1,
		BlockCompactionTrigger: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := vol.Delete(); err != nil {
			t.Error(err)
		}
	}()

	for _, data := range [][]byte{
		[]byte("maintenance-block-a"),
		[]byte("maintenance-block-b"),
	} {
		if _, _, err := vol.PutBlock(ctx, data, nil); err != nil {
			t.Fatal(err)
		}
	}

	before := loadTestBlockshardManifest(t, rootPath)
	if len(before.Segments) != 2 {
		t.Fatalf("segments before maintenance: got %d want 2 foreground publishes", len(before.Segments))
	}

	hooks, ok := vol.GetGCManagerHooks()
	if !ok || hooks.Maintenance == nil {
		t.Fatal("expected OPFS volume to expose blockshard maintenance hook")
	}
	if err := hooks.Maintenance(ctx); err != nil {
		t.Fatal(err)
	}

	after := loadTestBlockshardManifest(t, rootPath)
	if len(after.Segments) != 1 {
		t.Fatalf("segments after maintenance: got %d want 1 compacted segment", len(after.Segments))
	}
	if after.Segments[0].Level != 1 {
		t.Fatalf("compacted segment level=%d want 1", after.Segments[0].Level)
	}
}

func loadTestBlockshardManifest(t testing.TB, rootPath string) *blockshard.Manifest {
	t.Helper()

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	volDir, err := opfs.GetDirectory(root, rootPath, false)
	if err != nil {
		t.Fatal(err)
	}
	blocksDir, err := opfs.GetDirectory(volDir, "blocks", false)
	if err != nil {
		t.Fatal(err)
	}
	shardDir, err := opfs.GetDirectory(blocksDir, "shard-00", false)
	if err != nil {
		t.Fatal(err)
	}
	shard, err := blockshard.NewShard(0, shardDir, rootPath+"/blocks", nil)
	if err != nil {
		t.Fatal(err)
	}
	return shard.Manifest()
}
