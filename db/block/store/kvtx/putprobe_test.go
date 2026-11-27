package block_store_kvtx

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// TestPutProbeReplacesCorrupt reports whether a put of a hash whose stored
// bytes are corrupt replaces them, through PutBlock and PutBlockBatch.
func TestPutProbeReplacesCorrupt(t *testing.T) {
	ctx := context.Background()
	kvkey := store_kvkey.NewDefaultKVKey()
	store := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	blocks := NewKVTxBlock(kvkey, store, 0)
	good := []byte("repair probe block")
	bad := []byte("repair probe blocX")

	corrupt := func() *block.BlockRef {
		ref, _, err := blocks.PutBlock(ctx, good, nil)
		if err != nil {
			t.Fatal(err)
		}
		rm, err := ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, kvkey.GetBlockKey(rm), bad); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	read := func(ref *block.BlockRef) string {
		data, found, err := blocks.GetBlock(ctx, ref)
		switch {
		case err != nil:
			return "err=" + err.Error()
		case !found:
			return "absent"
		case bytes.Equal(data, good):
			return "good"
		case bytes.Equal(data, bad):
			return "corrupt bytes"
		}
		return "other"
	}

	ref := corrupt()
	fmt.Printf("PUTPROBE before put: %s\n", read(ref))
	_, existed, err := blocks.PutBlock(ctx, good, nil)
	fmt.Printf("PUTPROBE PutBlock existed=%v err=%v after: %s\n", existed, err, read(ref))

	ref = corrupt()
	ex, err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: good}})
	fmt.Printf("PUTPROBE PutBlockBatch existed=%v err=%v after: %s\n", ex, err, read(ref))
}
