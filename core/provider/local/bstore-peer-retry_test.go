package provider_local

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// flakyPeers fails its first fails reads, then answers from blk, which nil
// makes a miss. With stall set every read instead waits for its context and
// fails as the DEX fanout does, with ErrUnavailable around the context's error.
type flakyPeers struct {
	block.StoreOps
	fails int32
	stall bool
	reads atomic.Int32
	blk   *block.StoredBlock
}

// GetStoredBlock fails, then answers.
func (f *flakyPeers) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if f.stall {
		f.reads.Add(1)
		<-ctx.Done()
		return nil, fmt.Errorf("%w: %w", block.ErrUnavailable, ctx.Err())
	}
	if f.reads.Add(1) <= f.fails {
		return nil, errors.New("tier away on the peer")
	}
	return f.blk, nil
}

// peerRead reads peerReadRef through a peerMissLog over peers.
func peerRead(t *testing.T, peers block.StoreOps) (*block.StoredBlock, time.Duration, error) {
	t.Helper()
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), misses: &peerMisses{}}
	ref := peerReadRef(t)
	started := time.Now()
	blk, err := p.GetStoredBlock(context.Background(), ref)
	return blk, time.Since(started), err
}

// peerReadRef returns the ref peerRead reads.
func peerReadRef(t *testing.T) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte("peer read"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
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
// fails with block.ErrUnavailable naming the block, not a miss.
func TestPeerReadFailsUnavailable(t *testing.T) {
	SetPeerReadWait(300 * time.Millisecond)
	defer SetPeerReadWait(0)
	peers := &flakyPeers{fails: 1 << 30}
	blk, took, err := peerRead(t, peers)
	if blk != nil || !errors.Is(err, block.ErrUnavailable) || took > 2*time.Second {
		t.Fatalf("away: %v %v after %v", blk, err, took)
	}
	if !strings.Contains(err.Error(), peerReadRef(t).MarshalString()) {
		t.Fatalf("away: %v does not name the block", err)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Fatalf("away: %v reads as the caller's own context ending", err)
	}
	if peers.reads.Load() < 2 {
		t.Fatalf("%d reads, want retries", peers.reads.Load())
	}
}

// TestPeerReadBoundIsNotCallerDeadline checks that a peer read whose bound
// ends inside the peer store fails naming the block, and does not read as the
// caller's own deadline.
func TestPeerReadBoundIsNotCallerDeadline(t *testing.T) {
	SetPeerReadWait(200 * time.Millisecond)
	defer SetPeerReadWait(0)
	blk, _, err := peerRead(t, &flakyPeers{stall: true})
	if blk != nil || !errors.Is(err, block.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled: %v %v", blk, err)
	}
	if !strings.Contains(err.Error(), peerReadRef(t).MarshalString()) {
		t.Fatalf("stalled: %v does not name the block", err)
	}
}
