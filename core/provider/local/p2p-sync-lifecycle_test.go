package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/routine"
)

// TestRetrySharedObjectSyncRestartsOnlySelectedSO verifies that retry replaces
// one SO routine serially without disturbing its generation or sibling SOs.
func TestRetrySharedObjectSyncRestartsOnlySelectedSO(t *testing.T) {
	// Bound the selected shared-object retry.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Build sync routines that count concurrent executions.
	newSyncRoutine := func(started chan<- struct{}, running *atomic.Int32) *routine.RoutineContainer {
		// Start the counted sync routine and stop it on cleanup.
		rc := routine.NewRoutineContainer()
		rc.SetRoutine(func(ctx context.Context) error {
			// Announce the running sync and hold it until cancellation.
			if active := running.Add(1); active != 1 {
				t.Errorf("SO sync overlapped %d executions", active)
			}
			started <- struct{}{}
			<-ctx.Done()
			running.Add(-1)
			return ctx.Err()
		})
		rc.SetContext(ctx, false)
		t.Cleanup(func() {
			exitedCh, _ := rc.SetRoutine(nil)
			if exitedCh != nil {
				<-exitedCh
			}
		})
		return rc
	}

	// Start target and other shared-object routines.
	targetStarted := make(chan struct{}, 2)
	otherStarted := make(chan struct{}, 2)
	var targetRunning atomic.Int32
	var otherRunning atomic.Int32
	targetRoutine := newSyncRoutine(targetStarted, &targetRunning)
	otherRoutine := newSyncRoutine(otherStarted, &otherRunning)
	state := &p2pSyncState{
		ctx:     ctx,
		started: true,
		soSync: map[string]*accountObjectSync{
			"target": {sync: targetRoutine},
			"other":  {sync: otherRoutine},
		},
	}

	// Publish both shared-object routines on the account.
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.p2pSync = state
	})

	// Retry the target after both routines start.
	<-targetStarted
	<-otherStarted
	if !account.RetrySharedObjectSync("target") {
		t.Fatal("target SO sync was not restarted")
	}
	<-targetStarted

	// Require the other routine and the account sync state to stay unchanged.
	select {
	case <-otherStarted:
		t.Fatal("retry restarted an unrelated SO sync")
	default:
	}
	if targetRunning.Load() != 1 || otherRunning.Load() != 1 {
		t.Fatalf("unexpected active routines: target=%d other=%d", targetRunning.Load(), otherRunning.Load())
	}
	if account.p2pSync != state {
		t.Fatal("retry replaced the P2P transport generation")
	}
}

// TestRetrySharedObjectSyncRejectsUnknownSOWithoutRestart verifies that list
// reconciliation can still own initial startup when no routine exists yet.
func TestRetrySharedObjectSyncRejectsUnknownSOWithoutRestart(t *testing.T) {
	// Start a routine for the known shared object.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{}, 2)
	rc := routine.NewRoutineContainer()
	rc.SetRoutine(func(ctx context.Context) error {
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	rc.SetContext(ctx, false)
	t.Cleanup(func() {
		exitedCh, _ := rc.SetRoutine(nil)
		if exitedCh != nil {
			<-exitedCh
		}
	})

	// Publish the known shared-object routine on the account.
	state := &p2pSyncState{
		ctx:     ctx,
		started: true,
		soSync:  map[string]*accountObjectSync{"known": {sync: rc}},
	}
	account := &ProviderAccount{p2pSync: state}

	// Reject an unknown object without restarting the routine.
	<-started
	if account.RetrySharedObjectSync("missing") {
		t.Fatal("unknown SO sync was restarted")
	}
	select {
	case <-started:
		t.Fatal("failed retry restarted an existing SO sync")
	default:
	}
}

// TestRetireP2PSyncStateWaitsForSOSyncRoutine verifies that retirement joins
// SO routines before completing generation cleanup.
func TestRetireP2PSyncStateWaitsForSOSyncRoutine(t *testing.T) {
	// Hold a sync routine after cancellation until the test releases it.
	ctx, cancel := context.WithCancel(t.Context())
	routineStarted := make(chan struct{})
	routineCanceled := make(chan struct{})
	allowRoutineExit := make(chan struct{})
	var routineExited atomic.Bool
	rc := routine.NewRoutineContainer()
	rc.SetRoutine(func(ctx context.Context) error {
		// Announce start and cancellation before waiting for permission to exit.
		close(routineStarted)
		<-ctx.Done()
		close(routineCanceled)
		<-allowRoutineExit
		routineExited.Store(true)
		return ctx.Err()
	})
	rc.SetContext(ctx, false)

	// Publish the held routine on the account sync state.
	state := &p2pSyncState{
		ctx:           ctx,
		cancel:        cancel,
		started:       true,
		startComplete: true,
		startupExited: true,
		soSync:        map[string]*accountObjectSync{"target": {sync: rc}},
	}
	account := &ProviderAccount{p2pSync: state}

	// Retire the sync state and require retirement to wait for routine exit.
	<-routineStarted
	retired := make(chan struct{})
	go func() {
		account.retireP2PSyncState(nil)
		close(retired)
	}()
	<-routineCanceled
	select {
	case <-retired:
		t.Fatal("retirement completed before SO sync exited")
	default:
	}
	close(allowRoutineExit)
	<-retired
	if !routineExited.Load() {
		t.Fatal("retirement released resources before SO sync exited")
	}
}

func TestP2PSyncStateFinishStartPublishesStableState(t *testing.T) {
	// Build an unstarted sync state.
	ctx := t.Context()
	state := &p2pSyncState{ctx: ctx}

	// Finish startup and require no restart.
	restart := state.finishStart(nil)
	if restart {
		t.Fatal("stable startup requested a restart")
	}

	// Require the stable startup flags to be published.
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !state.startComplete || !state.started || state.startErr != nil {
			t.Fatalf("unexpected completed state: %+v", state)
		}
	})

	// Require a later canceled finish not to restart.
	restart = state.finishStart(context.Canceled)
	if restart {
		t.Fatal("completed startup requested a restart")
	}
}

func TestP2PSyncStateRestartStaysIncompleteUntilStable(t *testing.T) {
	// Build a sync state with a pending restart.
	ctx := t.Context()
	state := &p2pSyncState{ctx: ctx, restartPending: true}

	// Finish startup and require the pending restart.
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

	// Finish the stable restart without another restart request.
	restart = state.finishStart(nil)
	if restart {
		t.Fatal("stable startup requested an extra restart")
	}
}

func TestRetireP2PSyncStateWaitsForOwnedWork(t *testing.T) {
	// Build a sync state with owned work.
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

	// Retire the account sync in a background goroutine.
	retired := make(chan struct{})
	go func() {
		account.retireP2PSyncState(nil)
		close(retired)
	}()

	// Wait until the state is marked stopping.
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

	// Require retirement to wait for owned work.
	select {
	case <-retired:
		t.Fatal("retirement completed while owned work remained")
	default:
	}

	// Release owned work and wait for retirement.
	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		state.startupExited = true
		state.workers = 0
		bcast()
	})
	<-retired

	// Require the state to have released its lower source.
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !state.cleanupDone {
			t.Fatal("retirement did not complete cleanup")
		}
	})
}

func TestRetireP2PSyncStateReleasesLowerChain(t *testing.T) {
	// Build sync states with retained lower sources.
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

	// Install a three-state lower-source chain.
	first := makeState(nil)
	second := makeState(first)
	third := makeState(second)
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.p2pSync = third
	})

	// Retire the account sync and its lower chain.
	account.retireP2PSyncState(nil)

	// Require every state in the chain to retire.
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

func TestFailedP2PSyncStartDoesNotRestoreStoppedPrevious(t *testing.T) {
	// Build stopped sync states for the failed startup.
	makeState := func(stopping bool) *p2pSyncState {
		ctx, cancel := context.WithCancel(context.Background())
		return &p2pSyncState{
			ctx:           ctx,
			cancel:        cancel,
			startComplete: true,
			started:       !stopping,
			startupExited: true,
			stopping:      stopping,
		}
	}

	// Install the failed state after a stopped predecessor.
	previous := makeState(true)
	failed := makeState(true)
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.p2pSync = failed
	})

	// Attempt to restore the stopped predecessor.
	account.restoreP2PSyncAfterFailedStart(failed, previous, false)

	// Require neither state to be restored.
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if account.p2pSync != nil {
			t.Fatal("failed replacement restored a stopped previous state")
		}
	})
	previous.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !previous.cleanupDone {
			t.Fatal("stopped previous state was not retired")
		}
	})
}

func TestFailedP2PSyncStartDoesNotRestoreErroredPrevious(t *testing.T) {
	// Install a failed state after an errored predecessor.
	previousCtx, previousCancel := context.WithCancel(context.Background())
	previousCancel()
	previous := &p2pSyncState{
		ctx:           previousCtx,
		cancel:        previousCancel,
		startComplete: true,
		started:       true,
		startupExited: true,
	}
	failedCtx, failedCancel := context.WithCancel(context.Background())
	defer failedCancel()
	failed := &p2pSyncState{
		ctx:           failedCtx,
		cancel:        failedCancel,
		startComplete: true,
		startupExited: true,
		stopping:      true,
	}
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.p2pSync = failed
	})

	// Attempt to restore the errored predecessor.
	account.restoreP2PSyncAfterFailedStart(failed, previous, true)

	// Require neither state to be restored.
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if account.p2pSync != nil {
			t.Fatal("failed replacement restored a previous state with an errored context")
		}
	})
	previous.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !previous.cleanupDone {
			t.Fatal("previous state with an errored context was not retired")
		}
	})
}

func TestFailedP2PSyncStartDoesNotRestoreRetiredRetainedPredecessor(t *testing.T) {
	// Bound the retained-predecessor startup failure.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Build sync states with startup and ownership flags.
	makeState := func(owners int, complete bool) *p2pSyncState {
		stateCtx, stateCancel := context.WithCancel(ctx)
		return &p2pSyncState{
			ctx:           stateCtx,
			cancel:        stateCancel,
			owners:        owners,
			startComplete: complete,
			started:       complete,
			startupExited: true,
		}
	}

	// Install three overlapping startup states.
	oldest := makeState(1, true)
	middle := makeState(1, false)
	newest := makeState(1, false)
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Retain the oldest state as the middle state lower source.
		account.p2pSync = oldest
		if !account.retainP2PSyncLowerSourceLocked(oldest) {
			t.Fatal("middle start did not retain oldest state")
		}
		middle.lowerSource = oldest
		middle.lowerSourceHeld = true
		account.p2pSync = middle

		// Retain the middle state as the newest state lower source.
		if !account.retainP2PSyncLowerSourceLocked(middle) {
			t.Fatal("newest start did not retain middle state")
		}
		newest.lowerSource = middle
		newest.lowerSourceHeld = true
		account.p2pSync = newest
		bcast()
	})

	// The oldest caller gives up while the middle and newest starts still
	// retain the replacement chain.
	account.releaseP2PSyncState(oldest)

	// Retire the middle state while the newest state retains it.
	middleRetired := make(chan struct{})
	go func() {
		account.retireP2PSyncState(middle)
		close(middleRetired)
	}()
	select {
	case <-middleRetired:
	case <-ctx.Done():
		t.Fatal("middle state retirement did not complete")
	}
	select {
	case <-oldest.ctx.Done():
	case <-ctx.Done():
		t.Fatal("middle retirement did not stop oldest state")
	}
	oldest.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !oldest.stopping {
			t.Fatal("retired middle state left oldest state active")
		}
	})

	// Fail the newest startup and wait for its retirement.
	if restart := newest.finishStart(errors.New("controlled startup failure")); restart {
		t.Fatal("failed newest start requested a restart")
	}
	newest.markStartupExited()
	newestRetired := make(chan struct{})
	go func() {
		account.restoreP2PSyncAfterFailedStart(newest, middle, true)
		account.retireP2PSyncState(newest)
		close(newestRetired)
	}()
	select {
	case <-newestRetired:
	case <-ctx.Done():
		t.Fatal("newest state retirement did not complete")
	}

	// Require the retired predecessor not to become current.
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if account.p2pSync != nil {
			t.Fatal("failed newest start restored a retired predecessor")
		}
	})
}
