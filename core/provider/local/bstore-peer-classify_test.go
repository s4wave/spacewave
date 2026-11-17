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

// classStore is an indexed backend that places blocks as set.
type classStore struct {
	backendStore
	// placed is the index's answer.
	placed bool
	// unloaded reports the pack table not yet loaded.
	unloaded bool
	// locateErr fails the index lookup.
	locateErr error
	// hang holds the index lookup until its ctx ends.
	hang bool
	// bucket counts requests that would reach the bucket.
	bucket atomic.Int32
}

// LocateBlocks places each block as set.
func (s *classStore) LocateBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if s.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.locateErr != nil {
		return nil, s.locateErr
	}
	out := make([]bool, len(refs))
	for i := range out {
		out[i] = s.placed
	}
	return out, nil
}

// KeepsBlocks reports a backend that never reclaims.
func (s *classStore) KeepsBlocks() bool { return true }

// EntriesLoaded reports the pack table loaded unless set otherwise.
func (s *classStore) EntriesLoaded() bool { return !s.unloaded }

// GetBlockExists counts a request that would reach the bucket.
func (s *classStore) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	s.bucket.Add(1)
	return true, nil
}

// classifiedPeerRead has a peer serve a block to a placement over store with
// its classifier running, and returns the placement's classes once one counts.
func classifiedPeerRead(t *testing.T, store *classStore) PeerServedClasses {
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
	src := newPeerMissSource(logrus.NewEntry(logrus.New()), func() block.StoreOps { return peers }, nil, &p.peerReads)
	if blk, err := src().GetStoredBlock(ctx, block.NewBlockRef(nil)); blk == nil || err != nil {
		t.Fatalf("read: %v %v", blk, err)
	}

	// Wait for the classifier to count it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := p.readStatus().PeerServedClass
		if st.Indexed+st.NotIndexed+st.Unclassified != 0 {
			if n := store.bucket.Load(); n != 0 {
				t.Fatalf("classifier made %d bucket requests", n)
			}
			return st
		}
		if time.Now().After(deadline) {
			t.Fatal("served block never classified")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestPeerClassIndexed checks that a block the local index places counts as
// indexed without a bucket request.
func TestPeerClassIndexed(t *testing.T) {
	st := classifiedPeerRead(t, &classStore{placed: true})
	if st != (PeerServedClasses{Indexed: 1}) {
		t.Fatalf("classes: %+v", st)
	}
}

// TestPeerClassNotIndexed checks that a block the index places nowhere counts
// as not indexed.
func TestPeerClassNotIndexed(t *testing.T) {
	st := classifiedPeerRead(t, &classStore{})
	if st != (PeerServedClasses{NotIndexed: 1}) {
		t.Fatalf("classes: %+v", st)
	}
}

// TestPeerClassUnclassifiedUnloaded checks that a backend whose pack table is
// not loaded, so that a lookup would list the bucket, or whose lookup fails,
// leaves the block unclassified.
func TestPeerClassUnclassifiedUnloaded(t *testing.T) {
	if st := classifiedPeerRead(t, &classStore{placed: true, unloaded: true}); st != (PeerServedClasses{Unclassified: 1}) {
		t.Fatalf("unloaded classes: %+v", st)
	}
	if st := classifiedPeerRead(t, &classStore{placed: true, locateErr: errors.New("index away")}); st != (PeerServedClasses{Unclassified: 1}) {
		t.Fatalf("failed classes: %+v", st)
	}
}

// TestPeerClassUnclassifiedTimeout checks that a lookup outliving its bound
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
	src := newPeerMissSource(logrus.NewEntry(logrus.New()), func() block.StoreOps { return peers }, nil, &p.peerReads)
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
