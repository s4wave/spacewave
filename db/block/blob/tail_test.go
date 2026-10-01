package blob

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/util/prng"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestAppendTail appends a blob in small pieces, one transaction each, and
// checks that each append stores about its own bytes, that the chunks before
// the tail match the chunks built from the whole content, and that truncating
// into the tail and appending again keeps the content.
func TestAppendTail(t *testing.T) {
	// Build a store and the content.
	ctx := context.Background()
	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 3<<20)
	if _, err := io.ReadFull(prng.BuildSeededReader([]byte("append-tail")), data); err != nil {
		t.Fatal(err)
	}

	// Append the content in 32 KiB pieces, counting the bytes stored.
	wctx, counter := block.WithWriteCounter(ctx)
	var ref *block.BlockRef
	for off := 0; off < len(data); off += 32 << 10 {
		ref = appendTailStep(t, wctx, oc, ref, data[off:off+32<<10])
	}
	if written := counter.Snapshot().BlockWriteBytes; written > 3*uint64(len(data)) {
		t.Fatalf("stored %d bytes for %d bytes of content", written, len(data))
	}

	// Compare the content and the chunks before the tail.
	b := checkTailBlob(t, ctx, oc, ref, data)
	whole := buildWholeChunkIndex(t, ctx, oc, data)
	tailStart := b.GetChunkIndex().GetTailStart()
	if tailStart == 0 {
		t.Fatal("expected chunks before the tail")
	}
	for i, chk := range b.GetChunkIndex().GetChunks() {
		if chk.GetStart() >= tailStart {
			if chk.GetStart() == tailStart && whole[i].GetStart() != tailStart {
				t.Fatalf("tail start %d is not a chunk boundary", tailStart)
			}
			break
		}
		if !chk.EqualVT(whole[i]) {
			t.Fatalf("chunk %d differs from the whole-content chunk", i)
		}
	}

	// Truncate into the tail.
	nsize := len(data) - 100<<10
	btx, bcs := oc.BuildTransactionAtRef(nil, ref)
	b, err = UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Truncate(ctx, bcs, nil, int64(nsize)); err != nil {
		t.Fatal(err)
	}
	ref, _, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Append past the maximum chunk size so the truncated tail is chunked again.
	more := make([]byte, 200<<10)
	if _, err := io.ReadFull(prng.BuildSeededReader([]byte("append-tail-more")), more); err != nil {
		t.Fatal(err)
	}
	ref = appendTailStep(t, ctx, oc, ref, more)
	checkTailBlob(t, ctx, oc, ref, append(data[:nsize:nsize], more...))
}

// appendTailStep appends data to the blob at ref in one transaction.
func appendTailStep(t *testing.T, ctx context.Context, oc *bucket_lookup.Cursor, ref *block.BlockRef, data []byte) *block.BlockRef {
	// Open the blob, or start an empty one.
	t.Helper()
	btx, bcs := oc.BuildTransactionAtRef(nil, ref)
	b, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		b = &Blob{}
		bcs.SetBlock(b, true)
	}

	// Append and write the blob.
	if err := b.AppendData(ctx, int64(len(data)), bytes.NewReader(data), bcs, nil); err != nil {
		t.Fatal(err)
	}
	nref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return nref
}

// checkTailBlob checks that the blob at ref is valid and holds data.
func checkTailBlob(t *testing.T, ctx context.Context, oc *bucket_lookup.Cursor, ref *block.BlockRef, data []byte) *Blob {
	// Open and validate the blob.
	t.Helper()
	_, bcs := oc.BuildTransactionAtRef(nil, ref)
	b, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}

	// Read the content back.
	got, err := FetchToBytes(ctx, bcs)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("content differs: read %d bytes, expected %d", len(got), len(data))
	}
	return b
}

// buildWholeChunkIndex chunks data in one pass and returns its chunks.
func buildWholeChunkIndex(t *testing.T, ctx context.Context, oc *bucket_lookup.Cursor, data []byte) []*Chunk {
	// Build and write the blob from the whole content.
	t.Helper()
	btx, bcs := oc.BuildTransactionAtRef(nil, nil)
	b, err := BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), bcs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := btx.Write(ctx, true); err != nil {
		t.Fatal(err)
	}
	return b.GetChunkIndex().GetChunks()
}
