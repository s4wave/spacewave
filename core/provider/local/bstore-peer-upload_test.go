package provider_local

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_memcache "github.com/s4wave/spacewave/db/block/store/memcache"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
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

// newIndexedFallbackTest builds the placed store of newFallbackTest on a pack
// store that keeps a local pack index and never reclaims a pack, as the World's
// does, and opens it as the placement's backend.
func newIndexedFallbackTest(t *testing.T) *fallbackTest {
	// Reopen the pack store with an index every open shares.
	f := newFallbackTest(t)
	index := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	f.open = func() backendStore {
		store, err := buildS3BlockStore(logrus.NewEntry(logrus.New()), &account_settings.S3Location{
			Endpoint:   strings.TrimPrefix(f.srv.URL, "http://"),
			Region:     "us-east-1",
			Bucket:     "spaces",
			DisableSsl: true,
			KeepPacks:  true,
		}, "world", &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"}, index)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	f.packs.Close()
	f.packs = f.openPacks()

	// Open it as the placement's backend.
	f.p.remote = &atomic.Pointer[openBackend]{}
	f.p.remote.Store(&openBackend{backend: f.p.backend.Load(), store: f.packs})
	return f
}

// putUploaded writes a block, uploads it to the packs and evicts it, so only
// the packs hold it, and tracks it when tracked is set.
func (f *fallbackTest) putUploaded(data []byte, tracked bool) *block.BlockRef {
	f.t.Helper()
	ref, _, err := f.wb.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true})
	if err != nil {
		f.t.Fatal(err)
	}
	if tracked {
		f.tracked.track(ref)
	}
	f.upload()
	if result := f.evict(); result.Evicted != 1 || f.held(ref) {
		f.t.Fatalf("evict = %+v, want the block evicted", result)
	}
	return ref
}

// TestBackendHitsNeverQueue reads blocks the packs hold, tracked and
// untracked, through every read chain a placed store builds: with and without
// a read fallback, peer fills that upload, a fill cache and a front store. The
// peer and the fallback hold the blocks too, so a chain that asked either
// could fill them with a marker. No read asks them, and no block queues.
func TestBackendHitsNeverQueue(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, peerUpload := range []bool{false, true} {
			for _, cache := range []bool{false, true} {
				for _, front := range []bool{false, true} {
					name := fmt.Sprintf("fallback=%v/peer-upload=%v/cache=%v/front=%v", fallback, peerUpload, cache, front)
					t.Run(name, func(t *testing.T) {
						testBackendHitsNeverQueue(t, fallback, peerUpload, cache, front)
					})
				}
			}
		}
	}
}

// testBackendHitsNeverQueue runs TestBackendHitsNeverQueue on one chain.
func testBackendHitsNeverQueue(t *testing.T, fallback, peerUpload, cache, front bool) {
	// Put a tracked and an untracked block in the packs alone.
	f := newIndexedFallbackTest(t)
	tracked, untracked := []byte("tracked pack block"), []byte("untracked pack block")
	trackedRef := f.putUploaded(tracked, true)
	untrackedRef := f.putUploaded(untracked, false)

	// Give the peer and the fallback the same blocks.
	peer := &countingStore{StoreOps: newLeafStore()}
	for _, data := range [][]byte{tracked, untracked} {
		if _, _, err := peer.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
		f.putSQL(data)
	}

	// Build the chain.
	f.p.peerUpload = peerUpload
	if cache {
		f.p.cache = block_store_memcache.New(0)
	}
	if front {
		f.p.front = newLeafStore()
	}
	var fallbackStore block.StoreOps
	if fallback {
		fallbackStore = f.sql
	}
	recorded := &readRecorder{StoreOps: f.local, reads: &f.p.reads}
	f.reads, f.copies = f.p.readChains(
		recorded,
		f.local,
		func() block.StoreOps { return f.packs },
		func() block.StoreOps { return peer },
		fallbackStore,
	)

	// Read both; a chain that fills the disk lands them there.
	f.read(trackedRef, tracked)
	f.read(untrackedRef, untracked)
	if !cache && !front {
		f.waitHeld(trackedRef)
		f.waitHeld(untrackedRef)
	}
	if n := peer.reads.Load() + f.sql.reads.Load(); n != 0 {
		t.Fatalf("pack hits asked the peer or the fallback %d times", n)
	}
	if status := f.status(); status.Pending != 0 {
		t.Fatalf("pack hits queued %d uploads", status.Pending)
	}
}

// TestPlacedFillsNeverQueue fills blocks the pack index places from the
// fallback, after the bucket failed, and from a peer, after a pack read
// missed. Neither queues an upload. A block only the peer holds still does.
func TestPlacedFillsNeverQueue(t *testing.T) {
	t.Run("fallback", func(t *testing.T) {
		// Put a block in the packs and the fallback, then fail the bucket.
		f := newIndexedFallbackTest(t)
		data := []byte("pack block the fallback holds")
		ref := f.putUploaded(data, true)
		f.putSQL(data)
		f.bucket.setUnavailable(true)

		// The fallback serves it and fills the disk without a marker.
		f.read(ref, data)
		f.waitFallback(func(st FallbackStatus) bool { return st.Filled == 1 })
		f.waitHeld(ref)
		if status := f.status(); status.Pending != 0 {
			t.Fatalf("a placed fallback fill queued %d uploads", status.Pending)
		}
	})

	t.Run("peer", func(t *testing.T) {
		// Put a block in the packs, and read through packs that miss it, as
		// a pack read that misses a block its row names does.
		f := newIndexedFallbackTest(t)
		f.p.peerUpload = true
		data := []byte("pack block the peer holds")
		ref := f.putUploaded(data, false)
		peer := newLeafStore()
		if _, _, err := peer.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
		missing := newLeafStore()
		recorded := &readRecorder{StoreOps: f.local, reads: &f.p.reads}
		f.reads, f.copies = f.p.readChains(
			recorded,
			f.local,
			func() block.StoreOps { return missing },
			func() block.StoreOps { return peer },
			nil,
		)

		// The peer serves it and fills the disk without a marker.
		f.read(ref, data)
		f.waitHeld(ref)
		if status := f.status(); status.Pending != 0 {
			t.Fatalf("a placed peer fill queued %d uploads", status.Pending)
		}

		// A block the index does not place lands marked.
		peerData := []byte("block only the peer holds")
		peerRef, _, err := peer.PutBlock(f.ctx, peerData, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		f.read(peerRef, peerData)
		f.waitHeld(peerRef)
		deadline := time.Now().Add(10 * time.Second)
		for f.status().Pending != 1 {
			if time.Now().After(deadline) {
				t.Fatalf("pending = %d, want the peer's block", f.status().Pending)
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
}

// TestPeerFillsUploadUnderDivert checks the new owner's peer fills while its
// store diverts new values to a packahead Log: a block only the peer holds
// must still land with an upload marker and reach the pack store, as it does
// without the Log. The diverting store's values upload through their packs,
// but a peer fill is a cache fill the PackedStore keeps on the Volume, and
// nothing else uploads it.
func TestPeerFillsUploadUnderDivert(t *testing.T) {
	// Build the placed store over a diverting PackedStore.
	log, err := packahead.Open(t.Context(), t.TempDir(), packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	f := newFallbackTestWith(t, nil, log)

	// Rebuild the reads with a peer and peer fills that upload.
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

	// A block only the peer holds lands marked.
	peerData := []byte("old owner block, store diverting")
	peerRef, _, err := peer.PutBlock(f.ctx, peerData, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	f.read(peerRef, peerData)
	f.waitHeld(peerRef)
	deadline := time.Now().Add(5 * time.Second)
	for f.status().Pending != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("pending = %d, want the peer block queued to upload", f.status().Pending)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The upload moves it to a pack.
	f.upload()
	if !f.packHolds(peerRef) {
		t.Fatal("pack store lacks the peer block after the upload")
	}
}
