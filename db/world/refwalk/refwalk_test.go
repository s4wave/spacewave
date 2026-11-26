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
// refused real blobs would leave every payload blob unverified. It then
// checks the other side of the guess: bytes that decode as a valid blob and
// also hold a field no blob type defines, at any level, are another type's
// block, whose refs the blob decode skipped, so the walk reports them unsure
// with no refs and a rebuild writes no short list from them.
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

	// Build the same blob holding field 15, which no blob message defines,
	// in the blob and in each message below it that the blob package
	// defines.
	type message interface {
		MarshalVT() ([]byte, error)
		UnmarshalVT(data []byte) error
	}
	encode := func(msg message, suffix ...byte) []byte {
		t.Helper()
		data, err := msg.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		return append(data, suffix...)
	}
	unknown := []byte{15 << 3, 1}
	reread := func(msg, out message) {
		t.Helper()
		if err := out.UnmarshalVT(encode(msg, unknown...)); err != nil {
			t.Fatal(err)
		}
	}
	index, chunk, args := &blob.ChunkIndex{}, &blob.Chunk{}, &blob.ChunkerArgs{}
	rabin, jc := &blob.RabinArgs{}, &blob.JcArgs{}
	reread(b.GetChunkIndex(), index)
	reread(chunks[0], chunk)
	reread(b.GetChunkIndex().GetChunkerArgs(), args)
	reread(rabin.CloneVT(), rabin)
	reread(jc.CloneVT(), jc)
	edit := func(fn func(index *blob.ChunkIndex)) []byte {
		t.Helper()
		other := b.CloneVT()
		fn(other.ChunkIndex)
		return encode(other)
	}
	foreign := map[string][]byte{
		"blob":         encode(b, unknown...),
		"chunk index":  encode(&blob.Blob{BlobType: b.GetBlobType(), TotalSize: b.GetTotalSize(), ChunkIndex: index}),
		"chunk":        edit(func(index *blob.ChunkIndex) { index.Chunks[0] = chunk }),
		"chunker args": edit(func(index *blob.ChunkIndex) { index.ChunkerArgs = args }),
		"rabin args":   edit(func(index *blob.ChunkIndex) { index.ChunkerArgs = &blob.ChunkerArgs{RabinArgs: rabin} }),
		"jc args":      edit(func(index *blob.ChunkIndex) { index.ChunkerArgs = &blob.ChunkerArgs{JcArgs: jc} }),
	}

	// Each still names every chunk as a blob, and the walk trusts none.
	for name, data := range foreign {
		guessed := &blob.Blob{}
		if err := guessed.UnmarshalVT(data); err != nil || guessed.Validate() != nil || len(guessed.GetChunkIndex().GetChunks()) != len(chunks) {
			t.Fatalf("field in the %s: not a valid blob of %d chunks: %v", name, len(chunks), err)
		}
		foreignRef, _, err := store.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		node, ok, err := w.PayloadNode(&bucket.ObjectRef{}, foreignRef)
		if err != nil || !ok {
			t.Fatalf("field in the %s: payload node: ok %v, err %v", name, ok, err)
		}
		var got []Visit
		err = w.Walk(ctx, node, func(_ context.Context, batch []Visit) error {
			got = append(got, batch...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !got[0].Found || !got[0].Unsure || len(got[0].Refs) != 0 {
			t.Fatalf("field in the %s: visits %+v, want one unsure visit with no refs", name, got)
		}
	}
}
