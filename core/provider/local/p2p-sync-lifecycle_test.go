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
		ctx:   ctx,
		phase: p2pSyncRunning,
		soSync: map[string]*accountObjectSync{
			"target": {sync: targetRoutine},
			"other":  {sync: otherRoutine},
		},
	}

	// Publish both shared-object routines on the account.
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.setP2PSyncLocked(state)
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
		ctx:    ctx,
		phase:  p2pSyncRunning,
		soSync: map[string]*accountObjectSync{"known": {sync: rc}},
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

// newTestP2PSyncState builds a generation at phase. init sets its other fields
// before any goroutine sees it. Unless the generation is already Stopped, a
// stand-in for runP2PSync releases its resources once its context ends.
func newTestP2PSyncState(t *testing.T, account *ProviderAccount, phase p2pSyncPhase, init func(*p2pSyncState)) *p2pSyncState {
	ctx, cancel := context.WithCancel(context.Background())
	state := &p2pSyncState{ctx: ctx, cancel: cancel, phase: phase}
	if init != nil {
		init(state)
	}
	state.publishStoreViewLocked()
	if phase >= p2pSyncStopping {
		cancel()
	}
	if phase < p2pSyncStopped {
		go func() {
			<-ctx.Done()
			account.finishP2PSyncState(state)
		}()
		t.Cleanup(state.stop)
	}
	return state
}

// TestRetireP2PSyncStateWaitsForSOSyncRoutine verifies that retirement joins
// SO routines before completing generation cleanup.
func TestRetireP2PSyncStateWaitsForSOSyncRoutine(t *testing.T) {
	// Hold a sync routine after cancellation until the test releases it.
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

	// Publish the held routine on the account sync state.
	account := &ProviderAccount{}
	state := newTestP2PSyncState(t, account, p2pSyncRunning, func(s *p2pSyncState) {
		s.soSync = map[string]*accountObjectSync{"target": {sync: rc}}
	})
	rc.SetContext(state.ctx, false)
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.setP2PSyncLocked(state)
	})

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
	if phase := state.getPhase(); phase != p2pSyncStopped {
		t.Fatalf("retired state phase = %d, want Stopped", phase)
	}
}

// TestP2PSyncStateFinishStart verifies that a successful start enters Running
// once, and a failed one records its error and stops.
func TestP2PSyncStateFinishStart(t *testing.T) {
	// Finish a successful startup.
	account := &ProviderAccount{}
	state := newTestP2PSyncState(t, account, p2pSyncStarting, nil)
	if _, running := state.finishStart(nil); !running {
		t.Fatal("successful startup did not enter Running")
	}
	if _, running := state.finishStart(context.Canceled); running || state.getPhase() != p2pSyncRunning {
		t.Fatal("a second finish changed a running generation")
	}

	// Fail a startup.
	failErr := errors.New("controlled startup failure")
	failed := newTestP2PSyncState(t, account, p2pSyncStarting, nil)
	if _, running := failed.finishStart(failErr); running {
		t.Fatal("failed startup entered Running")
	}
	if err := failed.getStartErr(); err != failErr {
		t.Fatalf("start error = %v, want %v", err, failErr)
	}
	if err := failed.waitPhase(t.Context(), p2pSyncStopped); err != nil {
		t.Fatal(err)
	}
}

// TestRetireP2PSyncStateWaitsForRunGoroutine verifies that retirement returns
// only after the generation's run goroutine has released its resources.
func TestRetireP2PSyncStateWaitsForRunGoroutine(t *testing.T) {
	// Build a running generation whose run goroutine waits for the test.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var released atomic.Bool
	state := &p2pSyncState{
		ctx:    ctx,
		cancel: cancel,
		phase:  p2pSyncRunning,
		relFns: []func(){func() { released.Store(true) }},
	}
	account := &ProviderAccount{}
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.setP2PSyncLocked(state)
	})
	allowExit := make(chan struct{})
	go func() {
		<-ctx.Done()
		<-allowExit
		account.finishP2PSyncState(state)
	}()

	// Retire the account sync and wait until the state is stopping.
	retired := make(chan struct{})
	go func() {
		account.retireP2PSyncState(nil)
		close(retired)
	}()
	if err := state.waitPhase(t.Context(), p2pSyncStopping); err != nil {
		t.Fatal(err)
	}

	// Require retirement to wait for the run goroutine.
	select {
	case <-retired:
		t.Fatal("retirement completed while the run goroutine remained")
	default:
	}
	close(allowExit)
	<-retired
	if !released.Load() || state.getPhase() != p2pSyncStopped {
		t.Fatal("retirement did not complete cleanup")
	}
}

func TestRetireP2PSyncStateReleasesLowerChain(t *testing.T) {
	// Install a three-state lower-source chain: each successor's hold is its
	// predecessor's only owner.
	account := &ProviderAccount{}
	first := newTestP2PSyncState(t, account, p2pSyncRunning, func(s *p2pSyncState) { s.owners = 1 })
	second := newTestP2PSyncState(t, account, p2pSyncStarting, func(s *p2pSyncState) {
		s.owners, s.lowerSource = 1, first
	})
	third := newTestP2PSyncState(t, account, p2pSyncStarting, func(s *p2pSyncState) {
		s.owners, s.lowerSource = 1, second
	})
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.setP2PSyncLocked(third)
	})

	// Retire the account sync and its lower chain.
	account.retireP2PSyncState(nil)

	// Require every state in the chain to retire.
	for name, state := range map[string]*p2pSyncState{
		"first":  first,
		"second": second,
		"third":  third,
	} {
		if err := state.waitPhase(t.Context(), p2pSyncStopped); err != nil {
			t.Fatalf("%s state was not fully retired: %v", name, err)
		}
		state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if state.lowerSource != nil {
				t.Fatalf("%s state retained its lower source", name)
			}
		})
		if state.ctx.Err() == nil {
			t.Fatalf("%s state context was not canceled", name)
		}
	}
}

func TestFailedP2PSyncStartDoesNotRestoreStoppingPrevious(t *testing.T) {
	for _, retained := range []bool{false, true} {
		// Install a failed state after a stopping predecessor.
		account := &ProviderAccount{}
		previous := newTestP2PSyncState(t, account, p2pSyncStopping, nil)
		failed := newTestP2PSyncState(t, account, p2pSyncStopping, nil)
		account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			account.setP2PSyncLocked(failed)
		})

		// Attempt to restore the stopping predecessor.
		account.restoreP2PSyncAfterFailedStart(failed, previous, retained)

		// Require neither state to be restored, and the predecessor retired.
		account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if account.p2pSync != nil {
				t.Fatalf("failed replacement restored a stopping previous state (retained %v)", retained)
			}
		})
		if phase := previous.getPhase(); phase != p2pSyncStopped {
			t.Fatalf("stopping previous state phase = %d, want Stopped (retained %v)", phase, retained)
		}
	}
}

// TestFailedP2PSyncStartRestoresLivePrevious verifies that a failed replacement
// leaves a live retained predecessor current and running after its own cleanup
// releases the hold.
func TestFailedP2PSyncStartRestoresLivePrevious(t *testing.T) {
	// Install a starting replacement that holds a running predecessor.
	account := &ProviderAccount{}
	previous := newTestP2PSyncState(t, account, p2pSyncRunning, func(s *p2pSyncState) { s.owners = 2 })
	failed := newTestP2PSyncState(t, account, p2pSyncStarting, func(s *p2pSyncState) {
		s.owners, s.lowerSource = 1, previous
	})
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.setP2PSyncLocked(failed)
	})

	// Fail the replacement and restore the predecessor, as runP2PSync does.
	if _, running := failed.finishStart(errors.New("controlled startup failure")); running {
		t.Fatal("failed replacement entered Running")
	}
	account.restoreP2PSyncAfterFailedStart(failed, previous, true)
	if err := failed.waitPhase(t.Context(), p2pSyncStopped); err != nil {
		t.Fatal(err)
	}

	// Require the predecessor to stay current and running.
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if account.p2pSync != previous {
			t.Fatal("failed replacement did not restore its live predecessor")
		}
	})
	if phase := previous.getPhase(); phase != p2pSyncRunning {
		t.Fatalf("restored predecessor phase = %d, want Running", phase)
	}
}

func TestFailedP2PSyncStartDoesNotRestoreRetiredRetainedPredecessor(t *testing.T) {
	// Bound the retained-predecessor startup failure.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Install three overlapping generations: each holds its predecessor, which
	// also keeps its own caller.
	account := &ProviderAccount{}
	oldest := newTestP2PSyncState(t, account, p2pSyncRunning, func(s *p2pSyncState) { s.owners = 2 })
	middle := newTestP2PSyncState(t, account, p2pSyncStarting, func(s *p2pSyncState) {
		s.owners, s.lowerSource = 2, oldest
	})
	newest := newTestP2PSyncState(t, account, p2pSyncStarting, func(s *p2pSyncState) {
		s.owners, s.lowerSource = 1, middle
	})
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		account.setP2PSyncLocked(newest)
	})

	// The oldest caller gives up while the middle start still retains it.
	account.releaseP2PSyncState(oldest)

	// Retire the middle state while the newest state retains it.
	account.retireP2PSyncState(middle)
	if err := oldest.waitPhase(ctx, p2pSyncStopped); err != nil {
		t.Fatal("middle retirement did not stop oldest state")
	}

	// Fail the newest startup and restore, as runP2PSync does.
	if _, running := newest.finishStart(errors.New("controlled startup failure")); running {
		t.Fatal("failed newest start entered Running")
	}
	account.restoreP2PSyncAfterFailedStart(newest, middle, true)
	if err := newest.waitPhase(ctx, p2pSyncStopped); err != nil {
		t.Fatal("newest state retirement did not complete")
	}

	// Require the retired predecessor not to become current.
	account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if account.p2pSync != nil {
			t.Fatal("failed newest start restored a retired predecessor")
		}
	})
}
