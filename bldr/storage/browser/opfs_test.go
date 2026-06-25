//go:build js && !bldr_indexeddb

package browser_storage

import (
	"testing"

	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
)

func TestOpfsStorageBuildsV2RuntimeConfig(t *testing.T) {
	conf, err := NewOpfsStorage("prefix/").BuildVolumeConfig("state", nil)
	if err != nil {
		t.Fatal(err)
	}

	opfsConf, ok := conf.(*volume_opfs.Config)
	if !ok {
		t.Fatalf("BuildVolumeConfig returned %T, want *volume_opfs.Config", conf)
	}
	if opfsConf.GetSyncIo() {
		t.Fatal("SyncIo = true, want false")
	}
	if got, want := opfsConf.GetRootPath(), "prefix/state"; got != want {
		t.Fatalf("RootPath = %q, want %q", got, want)
	}
	if got, want := opfsConf.GetLockPrefix(), opfsConf.GetRootPath(); got != want {
		t.Fatalf("LockPrefix = %q, want %q", got, want)
	}
	if got, want := opfsConf.GetStorageFormatVersion(), uint32(2); got != want {
		t.Fatalf("StorageFormatVersion = %d, want %d", got, want)
	}
	if got, want := opfsConf.GetResetPolicy(), "automatic"; got != want {
		t.Fatalf("ResetPolicy = %q, want %q", got, want)
	}
	if got, want := opfsConf.GetDriverMode(), "auto"; got != want {
		t.Fatalf("DriverMode = %q, want %q", got, want)
	}
	if got, want := opfsConf.GetBlockShardCount(), uint32(blockshard.DefaultShardCount); got != want {
		t.Fatalf("BlockShardCount = %d, want %d", got, want)
	}
	if got, want := opfsConf.GetBlockCompactionTrigger(), uint32(8); got != want {
		t.Fatalf("BlockCompactionTrigger = %d, want %d", got, want)
	}
	if got, want := opfsConf.GetBlockMaxSegmentDataBytes(), uint32(blockshard.DefaultMaxSegmentDataBytes); got != want {
		t.Fatalf("BlockMaxSegmentDataBytes = %d, want %d", got, want)
	}
	if got, want := opfsConf.GetMetaShardCount(), uint32(1); got != want {
		t.Fatalf("MetaShardCount = %d, want %d", got, want)
	}
	if got, want := opfsConf.GetPageSize(), uint32(pagestore.DefaultPageSize); got != want {
		t.Fatalf("PageSize = %d, want %d", got, want)
	}
}
