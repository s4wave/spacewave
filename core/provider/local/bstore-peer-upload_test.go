package provider_local

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestPeerFillsUpload checks the new owner's peer fills: a block only the
// peer holds lands on the disk with an upload marker, eviction keeps it until
// the upload, and after the upload the pack store holds it; a block the packs
// hold still refills the disk without a marker.
func TestPeerFillsUpload(t *testing.T) {
	// Rebuild the reads with a peer and peer fills that upload.
	f := newFallbackTest(t)
	peer := newLeafStore()
	f.p.peerUpload = true
	recorded := &readRecorder{StoreOps: f.local, reads: &f.p.reads}
	f.reads, f.copies = f.p.readChains(
		recorded,
		f.local,
		func() block.StoreOps { return f.packs },
		func() block.StoreOps { return peer },
		nil,
	)

	// A pack hit refills the disk without a marker.
	packData := []byte("pack block")
	packRef, _, err := f.wb.PutBlock(f.ctx, packData, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	f.upload()
	if result := f.evict(); result.Evicted != 1 {
		t.Fatalf("evict = %+v, want the pack block evicted", result)
	}
	f.read(packRef, packData)
	f.waitHeld(packRef)
	if status := f.status(); status.Pending != 0 {
		t.Fatalf("pack hit queued %d uploads", status.Pending)
	}

	// A block only the peer holds lands marked.
	peerData := []byte("old owner block")
	peerRef, _, err := peer.PutBlock(f.ctx, peerData, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	f.read(peerRef, peerData)
	f.waitHeld(peerRef)
	deadline := time.Now().Add(10 * time.Second)
	for f.status().Pending != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("pending = %d, want the peer block", f.status().Pending)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Eviction keeps it until the upload moves it to a pack.
	f.evict()
	if !f.held(peerRef) || f.packHolds(peerRef) {
		t.Fatal("peer block evicted or packed before its upload")
	}
	f.upload()
	if !f.packHolds(peerRef) {
		t.Fatal("pack store lacks the peer block after the upload")
	}
	if result := f.evict(); result.Evicted != 1 || f.held(peerRef) {
		t.Fatalf("evict = %+v, want the uploaded peer block evicted", result)
	}
}
