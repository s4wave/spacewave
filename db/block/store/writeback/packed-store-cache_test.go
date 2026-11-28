package block_store_writeback

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackedStoreReadsEnterDecodedPool reads a block the Log holds through
// two transactions on one decoded-block cache: the first read admits it to
// the shared pool, as a Volume read does, and the second hits. After RmBlock
// forgets the Log's copy, a read serves no pooled block.
func TestPackedStoreReadsEnterDecodedPool(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	packed := NewPackedStore(log, ownedStore{env.vol}, true)

	// Divert a block to the Log.
	data, err := block_mock.NewExample("packed example").MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := packed.PutBlock(ctx, data, leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := env.vol.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("Volume holds the diverted bytes: %v %v", found, err)
	}

	// Isolate pool metrics from other scopes retained by the process-wide pool.
	cache, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)

	// read decodes ref in a fresh read-only transaction on cache.
	read := func() (block.Block, error) {
		btx, cursor := block.NewTransaction(packed, nil, ref, nil)
		btx.SetDecodedBlockCache(cache)
		btx.SetReadOnly()
		return cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
	}

	// The first read admits the block, and the second hits.
	if blk, err := read(); err != nil || blk.(*block_mock.Example).GetMsg() != "packed example" {
		t.Fatalf("first read = %v, %v", blk, err)
	}
	first := cache.Snapshot()
	if first.Stores == 0 {
		t.Fatalf("a Log read was not admitted to the pool: %+v", first)
	}
	if _, err := read(); err != nil {
		t.Fatal(err)
	}
	if second := cache.Snapshot(); second.Hits <= first.Hits {
		t.Fatalf("second read missed the pool: %+v", second)
	}

	// Remove the block and let its pack reach the remote.
	if err := packed.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	uploadTestPack(t, log, ref, data)

	// A fresh read must miss the pool and find no local copy.
	if blk, err := read(); blk != nil || !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("read after RmBlock and upload = %v, %v", blk, err)
	}
	if entries := cache.Snapshot().Entries; entries != 0 {
		t.Fatalf("the pool keeps %d entries after RmBlock", entries)
	}
}

// TestPackedStoreTombstoneForgetsLogCopy checks that a tombstone with diversion
// off keeps the Log's bytes readable until their pack reaches the remote.
func TestPackedStoreTombstoneForgetsLogCopy(t *testing.T) {
	// Build a PackedStore with a Log that can finalize its single pack.
	ctx := t.Context()
	env := newTestEnv(t)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)

	// Divert a block to the Log.
	data := []byte("packed, then tombstoned")
	ref, _, err := NewPackedStore(log, ownedStore{env.vol}, true).PutBlock(ctx, data, leafOpts)
	if err != nil {
		t.Fatal(err)
	}

	// Tombstone it through a store with the switch off.
	off := NewPackedStore(log, ownedStore{env.vol}, false)
	if _, err := off.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}}); err != nil {
		t.Fatal(err)
	}
	key := packfile.BlockKey(ref.GetHash())
	if log.HasDurable(key) {
		t.Fatal("a later put could skip the tombstoned block")
	}
	if got, found, err := off.GetBlock(ctx, ref); err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("pending tombstoned block = %q, %v, %v", got, found, err)
	}

	// The Log forgets the tombstoned value only after the remote has its bytes.
	uploadTestPack(t, log, ref, data)
	if log.Has(key) {
		t.Fatal("the Log keeps a tombstoned block after upload")
	}
	if _, found, err := off.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("tombstoned block reads locally after upload: %v, %v", found, err)
	}
}

// uploadTestPack drains the Log's one pack and checks the remote's copy of ref.
func uploadTestPack(t *testing.T, log *packahead.Log, ref *block.BlockRef, data []byte) {
	// Start a drain that keeps the uploaded pack on the local disk.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	remote := &packRemote{packs: make(map[string][]byte)}
	drain := packahead.NewDrain(log, packahead.DrainOptions{})
	done := make(chan error, 1)
	go func() { done <- drain.Run(ctx, remote) }()

	// Wait for the Log to publish the upload receipt, then join the drain.
	var uploaded []byte
	for {
		packs, changed := log.GetPacks()
		if len(packs) == 1 && packs[0].Uploaded {
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("stop drain: %v", err)
			}
			uploaded = remote.packs[packs[0].ID]
			break
		}
		select {
		case <-changed:
		case err := <-done:
			t.Fatalf("drain ended before uploading the pack: %v", err)
		}
	}

	// Read the exact value from the pack the remote accepted.
	key := packfile.BlockKey(ref.GetHash())
	size := uint64(len(uploaded))
	var found bool
	err := packfile.ScanPack(t.Context(), bytes.NewReader(uploaded), size, size, func(k, value []byte) error {
		if !bytes.Equal(k, key) {
			return nil
		}
		stored, err := block.DecodeBlockObject(value)
		if err != nil {
			return err
		}
		found = bytes.Equal(stored.Data, data)
		return nil
	})
	if err != nil || !found {
		t.Fatalf("uploaded pack did not contain %s: %v", ref.MarshalString(), err)
	}
}
