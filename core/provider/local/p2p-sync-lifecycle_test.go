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

	for {
		var (
			stopping bool
			waitCh   <-chan struct{}
		)
		state.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			stopping = state.stopping
			if !stopping {
				waitCh = getWaitCh()
			}
		})
		if stopping {
			break
		}
		<-waitCh
	}

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

func TestRetireP2PSyncStateReleasesLowerChain(t *testing.T) {
	makeState := func(lower *p2pSyncState) *p2pSyncState {
		ctx, cancel := context.WithCancel(context.Background())
		return &p2pSyncState{
			ctx:             ctx,
			cancel:          cancel,
			owners:          1,
			startComplete:   true,
			started:         true,
			startupExited:   true,
			lowerSource:     lower,
			lowerSourceHeld: lower != nil,
		}
	}

	first := makeState(nil)
	second := makeState(first)
	third := makeState(second)
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.p2pSync = third
	})

	account.retireP2PSyncState(nil)

	for name, state := range map[string]*p2pSyncState{
		"first":  first,
		"second": second,
		"third":  third,
	} {
		state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if !state.stopping || !state.cleanupDone {
				t.Fatalf("%s state was not fully retired", name)
			}
			if state.lowerSource != nil || state.lowerSourceHeld {
				t.Fatalf("%s state retained its lower source", name)
			}
		})
		if state.ctx.Err() == nil {
			t.Fatalf("%s state context was not canceled", name)
		}
	}
}
