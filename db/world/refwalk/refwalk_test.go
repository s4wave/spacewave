package refwalk

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/util/prng"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

// TestPayloadNodeBlob checks that a payload root guessed as a blob is
// confirmed when it is one: the walk keeps the decode and returns the refs of
// a chunked blob's chunks, so ref-repair verifies its list. A walk that
// refused real blobs would leave every payload blob unverified.
func TestPayloadNodeBlob(t *testing.T) {
	// Write a blob of several chunks with the blob package's builder.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(store, nil, nil, nil)
	data := make([]byte, 2<<20)
	if _, err := prng.BuildSeededReader([]byte("payload blob")).Read(data); err != nil {
		t.Fatal(err)
	}
	opts := &blob.BuildBlobOpts{RawHighWaterMark: 1}
	if _, err := blob.BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), cursor, opts); err != nil {
		t.Fatal(err)
	}
	ref, cursor, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := blob.UnmarshalBlob(ctx, cursor)
	if err != nil {
		t.Fatal(err)
	}
	chunks := b.GetChunkIndex().GetChunks()
	if len(chunks) < 2 {
		t.Fatalf("built %d chunks, want several", len(chunks))
	}

	// Walk the root as an operation payload no World typed.
	w := NewWalker(logrus.NewEntry(logrus.New()), store, nil)
	defer w.Close()
	root, ok, err := w.PayloadNode(&bucket.ObjectRef{}, ref)
	if err != nil || !ok {
		t.Fatalf("payload node: ok %v, err %v", ok, err)
	}
	var visits []Visit
	err = w.Walk(ctx, root, func(_ context.Context, batch []Visit) error {
		visits = append(visits, batch...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The root is certain and names every chunk.
	if len(visits) == 0 || visits[0].Node.Ref.MarshalString() != ref.MarshalString() {
		t.Fatal("walk did not visit the payload root first")
	}
	top := visits[0]
	if !top.Found || top.Unsure || top.DecodeErr != nil {
		t.Fatalf("root: found %v, unsure %v, decode err %v", top.Found, top.Unsure, top.DecodeErr)
	}
	refs := make(map[string]bool, len(top.Refs))
	for _, r := range top.Refs {
		refs[r.MarshalString()] = true
	}
	for i, c := range chunks {
		if !refs[c.GetDataRef().MarshalString()] {
			t.Fatalf("root refs lack chunk %d", i)
		}
	}
}
