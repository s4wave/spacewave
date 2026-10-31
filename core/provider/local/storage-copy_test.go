package provider_local

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// keyVolume is a key-value volume of which the list reads only the store and
// its keys.
type keyVolume struct {
	volume_kvtx.KvtxVolume
	store kvtx.Store
	kvkey *store_kvkey.KVKey
}

// GetKvtxStore returns the store.
func (v *keyVolume) GetKvtxStore() kvtx.Store { return v.store }

// GetKvKey returns the key layout.
func (v *keyVolume) GetKvKey() *store_kvkey.KVKey { return v.kvkey }

// TestListVolumeBlocks checks that the volume list returns every block key in
// order, in steps, and none of the other keys.
func TestListVolumeBlocks(t *testing.T) {
	// Store five block keys between keys outside the block prefix.
	ctx := t.Context()
	vol := &keyVolume{store: store_kvtx_inmem.NewStore(), kvkey: store_kvkey.NewDefaultKVKey()}
	var want []*block.BlockRef
	newTx := func(ctx context.Context) (kvtx.Tx, error) { return vol.store.NewTransaction(ctx, true) }
	err := kvtx.RunTransaction(ctx, true, newTx, func(ctx context.Context, tx kvtx.Tx) error {
		for _, data := range []string{"a", "b", "c", "d", "e"} {
			ref, err := block.BuildBlockRef([]byte(data), nil)
			if err != nil {
				return err
			}
			want = append(want, ref)
			key, _ := ref.MarshalVT()
			if err := tx.Set(ctx, vol.kvkey.GetBlockKey(key), []byte(data)); err != nil {
				return err
			}
		}
		for _, key := range []string{"", "\xff"} {
			if err := tx.Set(ctx, []byte(key+"other"), nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// List them two at a time.
	list := listVolumeBlocks(vol)
	var got []*block.BlockRef
	var after []byte
	for steps := 1; ; steps++ {
		refs, next, err := list(ctx, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, refs...)
		if next == nil {
			if steps != 3 {
				t.Fatalf("listed in %d steps, expected 3", steps)
			}
			break
		}
		after = next
	}
	if len(got) != len(want) {
		t.Fatalf("listed %d blocks, expected %d", len(got), len(want))
	}
	var prev []byte
	for _, ref := range got {
		key, _ := ref.MarshalVT()
		if bytes.Compare(prev, key) >= 0 {
			t.Fatal("blocks out of key order")
		}
		prev = key
	}
}
