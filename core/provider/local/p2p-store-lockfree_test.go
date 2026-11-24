package provider_local

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestGetP2PStoreWithoutLocks resolves a bucket's P2P store while another
// goroutine holds the account and generation locks: every block read
// resolves its lower store here, so a read never queues on a lifecycle lock.
// A replacement still starting serves its predecessor's stores.
func TestGetP2PStoreWithoutLocks(t *testing.T) {
	// Install a started generation with one store and a starting successor.
	var store block.StoreOps = &struct{ block.StoreOps }{}
	prior := &p2pSyncState{phase: p2pSyncRunning, stores: map[string]block.StoreOps{"b": store}}
	next := &p2pSyncState{lowerSource: prior}
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		prior.publishStoreViewLocked()
		next.publishStoreViewLocked()
		account.setP2PSyncLocked(next)
	})

	// Resolve the store while both locks are held.
	got := make(chan block.StoreOps, 1)
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		prior.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			next.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				go func() { got <- account.getP2PStore("b") }()
				select {
				case s := <-got:
					got <- s
				case <-time.After(10 * time.Second):
					t.Error("getP2PStore waited on a lifecycle lock")
				}
			})
		})
	})
	if s := <-got; s != store {
		t.Fatalf("getP2PStore = %v, want the predecessor's store", s)
	}

	// Once the successor starts, it stops serving the predecessor's stores.
	if _, running := next.finishStart(nil); !running {
		t.Fatal("successor did not enter Running")
	}
	if s := account.getP2PStore("b"); s != nil {
		t.Fatalf("started successor served %v, want nil", s)
	}
}
