package block_store_s3

import (
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/sirupsen/logrus"
)

// TestPackStoreMaxPackBytes checks a store with a raised ceiling: its batches
// hold half the ceiling, it writes one packfile past the 63 MiB default, and
// the blocks read back, while a store at the default refuses the same batch.
func TestPackStoreMaxPackBytes(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	const ceiling = 256 << 20
	big := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "big/", &PackStoreOptions{
		KeepPacks:    true,
		MaxPackBytes: ceiling,
	})
	t.Cleanup(big.Close)
	if limits := big.GetBatchLimits(); limits.Bytes != ceiling/2 {
		t.Fatalf("batch bytes = %d, want %d", limits.Bytes, ceiling/2)
	}

	// A batch of 72 MiB of incompressible blocks.
	var entries []*block.PutBatchEntry
	for range 72 {
		data := make([]byte, 1<<20)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data})
	}

	// The default store refuses it.
	if _, err := newTestPackStore(t, client).PutBlockBatch(ctx, entries); err == nil {
		t.Fatal("default store wrote a packfile past its ceiling")
	}

	// The raised store writes one packfile past the default and reads it back.
	if _, err := big.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	var sizes []int64
	if err := client.ListObjects(ctx, "bucket", "big/"+packDir, func(_ string, size int64) error {
		sizes = append(sizes, size)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 1 || sizes[0] <= writer.DefaultMaxPackBytes {
		t.Fatalf("pack sizes = %v, want one past the default ceiling", sizes)
	}
	reopened := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "big/", &PackStoreOptions{KeepPacks: true})
	t.Cleanup(reopened.Close)
	for _, e := range entries {
		data, found, err := reopened.GetBlock(ctx, e.Ref)
		if err != nil || !found || len(data) != len(e.Data) {
			t.Fatalf("read back = %d bytes, %v, %v", len(data), found, err)
		}
	}
}
