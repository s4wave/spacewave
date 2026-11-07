//go:build !tinygo

package block_store_s3

import (
	"path/filepath"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// indexVolumes are the volume types a backend keeps a pack index in, and the
// in-memory ordered map the unit tests use.
var indexVolumes = []string{"memory", "sqlite", "bolt", "s4db"}

// newIndexKV opens an empty pack index store of volume type kind, closed when
// the test ends.
func newIndexKV(t *testing.T, kind string) kvtx.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index")
	switch kind {
	case "memory":
		return hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]())
	case "sqlite":
		store, err := store_kvtx_sqlite.Open(t.Context(), path, "kv")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		return store
	case "bolt":
		store, err := store_kvtx_bolt.Open(path, 0o600, &bdb.Options{Timeout: time.Second}, []byte("hydra"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		return store
	case "s4db":
		db, err := s4db.Open(path, s4db.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	default:
		t.Fatalf("unknown index volume %q", kind)
		return nil
	}
}

// eachIndexVolume runs fn as a subtest for each volume type.
func eachIndexVolume(t *testing.T, fn func(t *testing.T, kind string)) {
	for _, kind := range indexVolumes {
		t.Run(kind, func(t *testing.T) { fn(t, kind) })
	}
}
