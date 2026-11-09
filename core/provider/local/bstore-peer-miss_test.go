package provider_local

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// flakyPeers fails its first fails reads, then answers from blk, which nil
// makes a miss.
type flakyPeers struct {
	block.StoreOps
	fails int32
	reads atomic.Int32
	blk   *block.StoredBlock
}

// GetStoredBlock fails, then answers.
func (f *flakyPeers) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if f.reads.Add(1) <= f.fails {
		return nil, errors.New("tier away on the peer")
	}
	return f.blk, nil
}

// peerRead reads ref through a peerMissLog over peers.
func peerRead(t *testing.T, peers block.StoreOps) (*block.StoredBlock, time.Duration, error) {
	t.Helper()
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), seen: &sync.Map{}}
	ref := block.NewBlockRef(nil)
	started := time.Now()
	blk, err := p.GetStoredBlock(context.Background(), ref)
	return blk, time.Since(started), err
}

// TestPeerReadRetriesFailures checks that a failed peer read asks again
// within the bound and returns the block a later read finds.
func TestPeerReadRetriesFailures(t *testing.T) {
	SetPeerReadWait(10 * time.Second)
	defer SetPeerReadWait(0)
	want := &block.StoredBlock{Data: []byte("served")}
	peers := &flakyPeers{fails: 3, blk: want}
	blk, _, err := peerRead(t, peers)
	if err != nil || blk != want {
		t.Fatalf("read: %v %v", blk, err)
	}
	if n := peers.reads.Load(); n != 4 {
		t.Fatalf("%d reads, want 4", n)
	}
}

// TestPeerReadMissIsFinal checks that a peer's answer that it lacks the block
// ends the read at once.
func TestPeerReadMissIsFinal(t *testing.T) {
	SetPeerReadWait(10 * time.Second)
	defer SetPeerReadWait(0)
	peers := &flakyPeers{}
	blk, took, err := peerRead(t, peers)
	if blk != nil || err != nil || took > time.Second || peers.reads.Load() != 1 {
		t.Fatalf("miss: %v %v after %v and %d reads", blk, err, took, peers.reads.Load())
	}
}

// TestPeerReadFailsUnavailable checks that a peer read failing past the bound
// fails with block.ErrUnavailable, not a miss.
func TestPeerReadFailsUnavailable(t *testing.T) {
	SetPeerReadWait(300 * time.Millisecond)
	defer SetPeerReadWait(0)
	peers := &flakyPeers{fails: 1 << 30}
	blk, took, err := peerRead(t, peers)
	if blk != nil || !errors.Is(err, block.ErrUnavailable) || took > 2*time.Second {
		t.Fatalf("away: %v %v after %v", blk, err, took)
	}
	if peers.reads.Load() < 2 {
		t.Fatalf("%d reads, want retries", peers.reads.Load())
	}
}

// hangingPeers answers nothing until ctx ends.
type hangingPeers struct {
	block.StoreOps
	asks atomic.Int32
}

// GetBlockExists waits for ctx.
func (h *hangingPeers) GetBlockExists(ctx context.Context, _ *block.BlockRef) (bool, error) {
	h.asks.Add(1)
	<-ctx.Done()
	return false, ctx.Err()
}

// GetBlockExistsBatch waits for ctx.
func (h *hangingPeers) GetBlockExistsBatch(ctx context.Context, _ []*block.BlockRef) ([]bool, error) {
	h.asks.Add(1)
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestPeerExistsAsksNoPeer checks that an existence probe, such as a put's,
// reports the block absent at once without asking a peer that never answers.
func TestPeerExistsAsksNoPeer(t *testing.T) {
	peers := &hangingPeers{}
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), seen: &sync.Map{}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ref := block.NewBlockRef(nil)
	if found, err := p.GetBlockExists(ctx, ref); found || err != nil {
		t.Fatalf("exists: %v %v", found, err)
	}
	if found, err := p.GetBlockExistsBatch(ctx, []*block.BlockRef{ref, ref}); len(found) != 2 || found[0] || found[1] || err != nil {
		t.Fatalf("exists batch: %v %v", found, err)
	}
	if ctx.Err() != nil || peers.asks.Load() != 0 {
		t.Fatalf("asked the peers %d times", peers.asks.Load())
	}
}

// TestPeerReadDeferred checks that a read under block.WithPeerReads asks no
// peer and fails unavailable at once, that the PeerReads' fetch reads the
// peer, and that the next read answers from the fetched block.
func TestPeerReadDeferred(t *testing.T) {
	want := &block.StoredBlock{Data: []byte("served")}
	peers := &flakyPeers{blk: want}
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), seen: &sync.Map{}}
	ref := block.NewBlockRef(nil)
	reads := block.NewPeerReads()
	ctx := block.WithPeerReads(context.Background(), reads)

	blk, err := p.GetStoredBlock(ctx, ref)
	if blk != nil || !errors.Is(err, block.ErrUnavailable) || peers.reads.Load() != 0 || !reads.Pending() {
		t.Fatalf("deferred read: %v %v, %d peer reads, pending %v", blk, err, peers.reads.Load(), reads.Pending())
	}
	ok, err := reads.Fetch(context.Background())
	if !ok || err != nil || peers.reads.Load() != 1 || reads.Pending() {
		t.Fatalf("fetch: %v %v, %d peer reads", ok, err, peers.reads.Load())
	}
	blk, err = p.GetStoredBlock(ctx, ref)
	if blk != want || err != nil || peers.reads.Load() != 1 {
		t.Fatalf("read after fetch: %v %v, %d peer reads", blk, err, peers.reads.Load())
	}
}
