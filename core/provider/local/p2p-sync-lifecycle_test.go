package provider_local

import (
	"context"
	"testing"
)

func TestP2PSyncStateFinishStartPublishesStableState(t *testing.T) {
	ctx := t.Context()
	state := &p2pSyncState{ctx: ctx}

	restart := state.finishStart(nil)
	if restart {
		t.Fatal("stable startup requested a restart")
	}

	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !state.startComplete || !state.started || state.startErr != nil {
			t.Fatalf("unexpected completed state: %+v", state)
		}
	})

	restart = state.finishStart(context.Canceled)
	if restart {
		t.Fatal("completed startup requested a restart")
	}
}

func TestP2PSyncStateRestartStaysIncompleteUntilStable(t *testing.T) {
	ctx := t.Context()
	state := &p2pSyncState{ctx: ctx, restartPending: true}

	restart := state.finishStart(nil)
	if !restart {
		t.Fatal("pending startup did not request a restart")
	}
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if state.startComplete {
			t.Fatal("restart decision published startup completion")
		}
		if state.restartPending {
			t.Fatal("restart decision remained pending")
		}
	})

	restart = state.finishStart(nil)
	if restart {
		t.Fatal("stable startup requested an extra restart")
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
