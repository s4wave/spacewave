package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// TestPeerReadsWaitAcrossRestart checks a placed store's peer source: before
// the bucket ever had peers a miss finds none; once it had them, a read while
// P2P sync restarts waits for the next lifecycle and reads the block from it,
// and a restart that does not return within the peer read bound reports the
// block unavailable, not missing.
func TestPeerReadsWaitAcrossRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Build an account and the peer source of one bucket.
		ctx := t.Context()
		account := &ProviderAccount{}
		source := newPeerMissSource(
			logrus.NewEntry(logrus.New()),
			func() block.StoreOps { return account.getP2PStore("b") },
			func(ctx context.Context) block.StoreOps { return account.waitP2PStore(ctx, "b") },
		)
		if source() != nil {
			t.Fatal("a source before any P2P sync reads peers")
		}

		// A peer store holds the block.
		peers := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
		ref, _, err := peers.PutBlock(ctx, []byte("peer block"), nil)
		if err != nil {
			t.Fatal(err)
		}
		lifecycle := func(store block.StoreOps) *p2pSyncState {
			state := &p2pSyncState{started: true, stores: map[string]block.StoreOps{"b": store}}
			account.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
				state.publishStoreViewLocked()
				account.setP2PSyncLocked(state)
				bcast()
			})
			return state
		}
		stop := func() {
			account.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
				account.setP2PSyncLocked(nil)
				bcast()
			})
		}
		lifecycle(peers)
		if source() == nil {
			t.Fatal("a source with peers reads none")
		}
		stop()

		// A read during the restart waits for the next lifecycle.
		read := make(chan error, 1)
		go func() {
			data, found, err := source().GetBlock(ctx, ref)
			if err == nil && (!found || string(data) != "peer block") {
				err = errors.New("read the wrong block")
			}
			read <- err
		}()
		synctest.Wait()
		select {
		case err := <-read:
			t.Fatalf("a read during the restart returned before it: %v", err)
		default:
		}
		lifecycle(peers)
		if err := <-read; err != nil {
			t.Fatal(err)
		}

		// A restart that does not return within the bound is unavailable.
		stop()
		if _, _, err := source().GetBlock(ctx, ref); !errors.Is(err, block.ErrUnavailable) {
			t.Fatalf("a read past the restart bound = %v, want unavailable", err)
		}
	})
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
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), misses: &peerMisses{}}
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
