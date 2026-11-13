package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// classStore is an indexed backend that places, and confirms, as set.
type classStore struct {
	backendStore
	// placed is the index's answer.
	placed bool
	// held is the packs' answer.
	held bool
	// existsErr fails the packs' check.
	existsErr error
	// hang holds the packs' check until its ctx ends.
	hang bool
}

// LocateBlocks places each block as set.
func (s *classStore) LocateBlocks(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i := range out {
		out[i] = s.placed
	}
	return out, nil
}

// KeepsBlocks reports a backend that never reclaims.
func (s *classStore) KeepsBlocks() bool { return true }

// GetBlockExists answers the packs' check as set.
func (s *classStore) GetBlockExists(ctx context.Context, _ *block.BlockRef) (bool, error) {
	if s.hang {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return s.held, s.existsErr
}

// classifiedPeerRead has a peer serve a block to a placement over store with
// its classifier running, and returns the placement's classes once one counts.
func classifiedPeerRead(t *testing.T, store backendStore) PeerServedClasses {
	t.Helper()
	SetPeerReadWait(10 * time.Second)
	defer SetPeerReadWait(0)

	// Place the store over the backend.
	backend := &account_settings.StorageBackend{}
	p := &placementState{remote: &atomic.Pointer[openBackend]{}}
	p.backend.Store(backend)
	p.remote.Store(&openBackend{backend: backend, store: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startPeerClassify(ctx)

	// Read a block a peer serves.
	peers := &flakyPeers{blk: &block.StoredBlock{Data: []byte("served")}}
	src := newPeerMissSource(logrus.NewEntry(logrus.New()), func() block.StoreOps { return peers }, &p.peerReads)
	if blk, err := src().GetStoredBlock(ctx, block.NewBlockRef(nil)); blk == nil || err != nil {
		t.Fatalf("read: %v %v", blk, err)
	}

	// Wait for the classifier to count it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := p.readStatus().PeerServedClass
		if st.IndexedHeld+st.IndexedMissing+st.NotIndexed+st.Unclassified != 0 {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatal("served block never classified")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestPeerClassIndexedHeld checks that a block the index places and the packs
// confirm counts as indexed and held.
func TestPeerClassIndexedHeld(t *testing.T) {
	st := classifiedPeerRead(t, &classStore{placed: true, held: true})
	if st != (PeerServedClasses{IndexedHeld: 1}) {
		t.Fatalf("classes: %+v", st)
	}
}

// TestPeerClassIndexedMissing checks that a block the index places but the
// packs deny, or fail to check, counts as indexed and missing.
func TestPeerClassIndexedMissing(t *testing.T) {
	if st := classifiedPeerRead(t, &classStore{placed: true}); st != (PeerServedClasses{IndexedMissing: 1}) {
		t.Fatalf("denied classes: %+v", st)
	}
	if st := classifiedPeerRead(t, &classStore{placed: true, existsErr: errors.New("bucket away")}); st != (PeerServedClasses{IndexedMissing: 1}) {
		t.Fatalf("failed classes: %+v", st)
	}
}

// TestPeerClassNotIndexed checks that a block the index places nowhere counts
// as not indexed.
func TestPeerClassNotIndexed(t *testing.T) {
	st := classifiedPeerRead(t, &classStore{held: true})
	if st != (PeerServedClasses{NotIndexed: 1}) {
		t.Fatalf("classes: %+v", st)
	}
}

// TestPeerClassUnclassifiedTimeout checks that a check outliving its bound
// leaves the block unclassified.
func TestPeerClassUnclassifiedTimeout(t *testing.T) {
	st := classifiedPeerRead(t, &classStore{placed: true, hang: true})
	if st != (PeerServedClasses{Unclassified: 1}) {
		t.Fatalf("classes: %+v", st)
	}
}

// TestPeerClassUnclassifiedQueueFull checks that a served block the full queue
// drops counts unclassified without the read waiting.
func TestPeerClassUnclassifiedQueueFull(t *testing.T) {
	p := &placementState{remote: &atomic.Pointer[openBackend]{}}
	p.peerReads.queue = make(chan *block.BlockRef)
	peers := &flakyPeers{blk: &block.StoredBlock{Data: []byte("served")}}
	src := newPeerMissSource(logrus.NewEntry(logrus.New()), func() block.StoreOps { return peers }, &p.peerReads)
	if blk, err := src().GetStoredBlock(context.Background(), block.NewBlockRef(nil)); blk == nil || err != nil {
		t.Fatalf("read: %v %v", blk, err)
	}
	if st := p.readStatus(); st.PeerServed != 1 || st.PeerServedClass != (PeerServedClasses{Unclassified: 1}) {
		t.Fatalf("status: %+v", st)
	}
}

// TestPeerClassServeOnly checks that serve mode classifies nothing.
func TestPeerClassServeOnly(t *testing.T) {
	p := &placementState{serveOnly: true}
	p.startPeerClassify(context.Background())
	if p.peerReads.queue != nil {
		t.Fatal("serve mode started a classifier")
	}
}
