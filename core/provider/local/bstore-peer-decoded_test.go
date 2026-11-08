package provider_local

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestPeerBlockWithoutRefsKeptOnDecode checks that a block a peer serves
// without its refs is kept, with an upload marker, once a cursor decodes it,
// with the refs the decoded block holds; and that a raw read, which decodes
// nothing, keeps nothing.
func TestPeerBlockWithoutRefsKeptOnDecode(t *testing.T) {
	// Rebuild the reads with a peer that serves no refs, and peer fills that
	// upload.
	f := newFallbackTest(t)
	peer := &refsUnknownStore{StoreOps: block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)}
	f.p.peerUpload = true
	recorded := &readRecorder{StoreOps: f.local, reads: &f.p.reads}
	f.reads, f.copies = f.p.readChains(
		recorded,
		f.local,
		func() block.StoreOps { return f.packs },
		func() block.StoreOps { return peer },
		nil,
	)

	// The peer holds a pair of two children.
	put := func(b block.Block) *block.BlockRef {
		data, err := b.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		ref, _, err := peer.PutBlock(f.ctx, data, &block.PutOpts{})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	left := put(block_mock.NewExample("left"))
	right := put(block_mock.NewExample("right"))
	pairRef := put(&block_mock.Pair{Left: left, Right: right, Msg: "pair"})
	rawRef := put(&block_mock.Pair{Left: left, Msg: "raw"})

	// A raw read decodes nothing, so it keeps nothing.
	if _, found, err := f.reads.GetBlock(f.ctx, rawRef); err != nil || !found {
		t.Fatalf("raw read = %v, %v", found, err)
	}

	// A cursor's decode keeps the pair with its children as refs.
	_, cursor := block.NewTransaction(f.reads, nil, pairRef, nil)
	if _, err := cursor.Unmarshal(f.ctx, block_mock.NewPairBlock); err != nil {
		t.Fatal(err)
	}
	f.waitHeld(pairRef)
	stored, err := f.local.GetStoredBlock(f.ctx, pairRef)
	if err != nil || stored == nil {
		t.Fatalf("kept pair = %v, %v", stored, err)
	}
	if !stored.RefsKnown || len(stored.Refs) != 2 {
		t.Fatalf("kept pair refs known=%v refs=%d, want 2 known", stored.RefsKnown, len(stored.Refs))
	}
	want := map[string]bool{left.MarshalString(): true, right.MarshalString(): true}
	for _, r := range stored.Refs {
		if !want[r.MarshalString()] {
			t.Fatalf("kept pair has ref %s, not a child", r.MarshalString())
		}
	}

	// The kept pair waits for its upload.
	deadline := time.Now().Add(10 * time.Second)
	for f.status().Pending != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("pending = %d, want the pair", f.status().Pending)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if f.held(rawRef) {
		t.Fatal("raw read kept a block without refs")
	}
}
