//go:build darwin || linux || windows

package kvtx

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestUpgradeBase58BlockKeys opens a store whose blocks are keyed in base58,
// as version 0 wrote them, with one block already moved by an interrupted
// migration, and expects every block under its binary key.
func TestUpgradeBase58BlockKeys(t *testing.T) {
	t.Run("inmem", func(t *testing.T) {
		testUpgradeBase58BlockKeys(t, store_kvtx_inmem.NewStore())
	})
	t.Run("s4db", func(t *testing.T) {
		db, err := s4db.Open(filepath.Join(t.TempDir(), "volume.s4db"), s4db.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		testUpgradeBase58BlockKeys(t, db)
	})
}

// testUpgradeBase58BlockKeys runs TestUpgradeBase58BlockKeys on one store.
func testUpgradeBase58BlockKeys(t *testing.T, store db_kvtx.Store) {
	// Prepare the blocks of the unversioned store.
	ctx := context.Background()
	keys := store_kvkey.NewDefaultKVKey()
	blocks := [][]byte{[]byte("alpha"), []byte("beta"), []byte("gamma")}
	refs := make([]*block.BlockRef, len(blocks))

	// Write version 0 blocks, leaving the last one moved.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i, data := range blocks {
		refs[i], err = block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		rm, err := refs[i].MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		key := keys.GetBlockKey(rm)
		if i != len(blocks)-1 {
			key = append(keys.GetBlockFullPrefix(), b58.Encode(rm)...)
		}
		if err := tx.Set(ctx, key, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Open the Volume, which migrates the store.
	vol, err := NewVolume(ctx, "hydra/test-volume", keys, store, nil, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	// Every block reads back.
	for i, ref := range refs {
		data, found, err := vol.GetBlock(ctx, ref)
		if err != nil || !found || !bytes.Equal(data, blocks[i]) {
			t.Fatalf("block %d: found %v data %q err %v", i, found, data, err)
		}
	}

	// Only binary block keys remain.
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	var count int
	err = rtx.ScanPrefixKeys(ctx, keys.GetBlockFullPrefix(), func(key []byte) error {
		count++
		if key[len(keys.GetBlockFullPrefix())] != binaryBlockRefTag {
			t.Errorf("block key %q is not binary", key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != len(blocks) {
		t.Fatalf("store holds %d block keys, want %d", count, len(blocks))
	}

	// The store records the current version.
	version, err := readFormatVersion(ctx, store, keys)
	if err != nil {
		t.Fatal(err)
	}
	if version != store_kvkey.FormatVersion {
		t.Fatalf("format version %d, want %d", version, store_kvkey.FormatVersion)
	}
}
