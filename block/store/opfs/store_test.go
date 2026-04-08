//go:build js

package block_store_opfs

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/opfs"
)

func TestBlockStorePutGet(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-blockstore-basic", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-blockstore-basic", true) //nolint

	ctx := context.Background()
	bs := NewBlockStore(dir, "test-blockstore-basic", hash.HashType_HashType_BLAKE3)

	data := []byte("hello block store")
	ref, exists, err := bs.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected new block, got exists=true")
	}

	// Put again: should be idempotent.
	ref2, exists2, err := bs.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !exists2 {
		t.Fatal("expected exists=true on second put")
	}
	if !ref.EqualsRef(ref2) {
		t.Fatal("refs differ on second put")
	}

	// Get.
	got, found, err := bs.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("block not found")
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}

	// Exists.
	ok, err := bs.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("GetBlockExists returned false")
	}

	// Stat.
	stat, err := bs.StatBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if stat == nil {
		t.Fatal("StatBlock returned nil")
	}
	if stat.Size != int64(len(data)) {
		t.Fatalf("stat size = %d, want %d", stat.Size, len(data))
	}

	// Remove.
	if err := bs.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	ok, err = bs.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("block still exists after remove")
	}
}

func TestBlockStoreConcurrentPut(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-blockstore-conc", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-blockstore-conc", true) //nolint

	ctx := context.Background()
	bs := NewBlockStore(dir, "test-blockstore-conc", hash.HashType_HashType_BLAKE3)

	// Put distinct blocks concurrently.
	const n = 10
	refs := make([]*block.BlockRef, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			data := []byte("block-" + strconv.Itoa(i))
			ref, _, err := bs.PutBlock(ctx, data, nil)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			refs[i] = ref
			mu.Unlock()
		}()
	}
	wg.Wait()
	if t.Failed() {
		return
	}

	// Verify all blocks readable.
	for i := range n {
		data, found, err := bs.GetBlock(ctx, refs[i])
		if err != nil {
			t.Fatalf("block %d: %v", i, err)
		}
		if !found {
			t.Fatalf("block %d not found", i)
		}
		want := []byte("block-" + strconv.Itoa(i))
		if !bytes.Equal(data, want) {
			t.Fatalf("block %d: got %q, want %q", i, data, want)
		}
	}

	// Put the SAME block concurrently from multiple goroutines.
	sameData := []byte("same-block-data")
	var wg2 sync.WaitGroup
	wg2.Add(n)
	for range n {
		go func() {
			defer wg2.Done()
			_, _, err := bs.PutBlock(ctx, sameData, nil)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg2.Wait()
}
