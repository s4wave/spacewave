package provider_local

import (
	"context"
	"testing"
)

func TestP2PSyncStateFinishStartPublishesStableState(t *testing.T) {
	ctx := t.Context()
	state := &p2pSyncState{ctx: ctx}

	restart, published, err := state.finishStart(nil)
	if restart || !published || err != nil {
		t.Fatalf("finish start = restart %v, published %v, err %v", restart, published, err)
	}

	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !state.startComplete || !state.started || state.startErr != nil {
			t.Fatalf("unexpected completed state: %+v", state)
		}
	})

	restart, published, err = state.finishStart(context.Canceled)
	if restart || published || err != nil {
		t.Fatalf("second finish start = restart %v, published %v, err %v", restart, published, err)
	}
}

func TestP2PSyncStateRestartStaysIncompleteUntilStable(t *testing.T) {
	ctx := t.Context()
	state := &p2pSyncState{ctx: ctx, restartPending: true}

	restart, published, err := state.finishStart(nil)
	if !restart || published || err != nil {
		t.Fatalf("pending finish start = restart %v, published %v, err %v", restart, published, err)
	}
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if state.startComplete {
			t.Fatal("restart decision published startup completion")
		}
		if state.restartPending {
			t.Fatal("restart decision remained pending")
		}
	})

	restart, published, err = state.finishStart(nil)
	if restart || !published || err != nil {
		t.Fatalf("stable finish start = restart %v, published %v, err %v", restart, published, err)
	}
}

func TestRetireP2PSyncStateWaitsForOwnedWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &p2pSyncState{
		ctx:           ctx,
		cancel:        cancel,
		startupExited: false,
		workers:       1,
		relFns:        []func(){func() {}},
	}
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.p2pSync = state
	})

	retired := make(chan struct{})
	go func() {
		account.retireP2PSyncState(nil)
		close(retired)
	}()

	var stoppingCh <-chan struct{}
	state.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		stoppingCh = getWaitCh()
	})
	<-stoppingCh

	select {
	case <-retired:
		t.Fatal("retirement completed while owned work remained")
	default:
	}

	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		state.startupExited = true
		state.workers = 0
		bcast()
	})
	<-retired

	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !state.cleanupDone {
			t.Fatal("retirement did not complete cleanup")
		}
	})
}
