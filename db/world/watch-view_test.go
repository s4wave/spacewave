package world_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestWatchViewSecondTransaction checks a lost wakeup when only a transaction
// opened by a captured client reads the changed key, before the watch starts.
func TestWatchViewSecondTransaction(t *testing.T) {
	// Give the view and its captured transaction client the same tracking engine.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	engine := world.NewReadTrackingEngine(tb.Engine)

	// Coordinate the write with the evaluation before its change wait begins.
	started := make(chan struct{})
	committed := make(chan struct{})
	done := make(chan error, 1)
	stop := errors.New("view observed second transaction change")
	runs := 0

	// Read the first key through the supplied state and the second through a client.
	go func() {
		done <- world.WatchView(ctx, engine, func(ctx context.Context, ws world.WorldState) error {
			// Keep the first dependency unchanged throughout the test.
			if _, err := ws.HasObject(ctx, "view/first"); err != nil {
				return err
			}

			// Open and discard a distinct snapshot through the captured client.
			found, err := readViewObject(ctx, engine, "view/second")
			if err != nil {
				return err
			}
			runs++
			if runs == 2 {
				if !found {
					return errors.New("rerun did not read the second transaction's change")
				}
				return stop
			}

			// Let the write land after both reads but before WaitChange starts.
			close(started)
			select {
			case <-committed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	// Change only the second transaction's key while the first evaluation runs.
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("view did not finish its two initial reads")
	}
	createViewObject(t, ctx, engine, "view/second")
	close(committed)

	// Exactly one rerun must observe the change without another commit.
	select {
	case err := <-done:
		if !errors.Is(err, stop) {
			t.Fatalf("WatchView = %v, want %v", err, stop)
		}
		if runs != 2 {
			t.Fatalf("view ran %d times, want 2", runs)
		}
	case <-ctx.Done():
		t.Fatal("view missed the second transaction's change")
	}
}

// TestWatchViewRerunAt checks the earliest deadline without any World commits.
func TestWatchViewRerunAt(t *testing.T) {
	// Bound a quiet World view and its requested deadline.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	deadline := time.Now().Add(20 * time.Millisecond)
	stop := errors.New("deadline reached")
	runs := 0

	// Request deadlines in both orders; neither a later nor zero time replaces it.
	err := world.WatchView(ctx, tb.WorldState, func(ctx context.Context, _ world.WorldState) error {
		// Finish the second evaluation only after the requested time.
		runs++
		if runs == 2 {
			if time.Now().Before(deadline) {
				return errors.New("view reran before its deadline")
			}
			return stop
		}

		// Register the earliest deadline among later and absent requests.
		world.RerunAt(ctx, deadline.Add(time.Hour))
		world.RerunAt(ctx, deadline)
		world.RerunAt(ctx, deadline.Add(2*time.Hour))
		world.RerunAt(ctx, time.Time{})
		return nil
	})

	// A timer, not a commit or caller cancellation, must have rerun the view.
	if !errors.Is(err, stop) {
		t.Fatalf("WatchView = %v, want %v", err, stop)
	}
	if runs != 2 {
		t.Fatalf("view ran %d times, want 2", runs)
	}
}

// TestWatchViewRerunOn checks that a view reruns when a context it recorded
// ends, also when it recorded the context in a shared set it joined.
func TestWatchViewRerunOn(t *testing.T) {
	// Bound a quiet World view.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	stop := errors.New("rerun")
	runs := 0

	// Record one context directly and one in a shared set, ending each in
	// turn after its run returns.
	direct, endDirect := context.WithCancel(ctx)
	defer endDirect()
	shared, endShared := context.WithCancel(ctx)
	defer endShared()
	err := world.WatchView(ctx, tb.WorldState, func(ctx context.Context, _ world.WorldState) error {
		runs++
		switch runs {
		case 1:
			world.RerunOn(ctx, direct)
			time.AfterFunc(10*time.Millisecond, endDirect)
		case 2:
			set := world.NewReadSet()
			child, finish := world.WithViewReads(ctx, set)
			world.RerunOn(child, shared)
			world.AddViewReads(ctx, set, finish())
			time.AfterFunc(10*time.Millisecond, endShared)
		default:
			return stop
		}
		return nil
	})

	// Only the ended contexts, not a commit or a deadline, reran the view.
	if !errors.Is(err, stop) {
		t.Fatalf("WatchView = %v, want %v", err, stop)
	}
	if runs != 3 {
		t.Fatalf("view ran %d times, want 3", runs)
	}
}

// TestSendViewsSuppressesEqual checks first-nil delivery and generated equality.
func TestSendViewsSuppressesEqual(t *testing.T) {
	// Prepare immutable projections, including equal but distinct messages.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	changed := block_mock.NewExample("changed")
	projections := []*block_mock.Example{nil, nil, changed, changed.CloneVT()}
	stop := errors.New("all projections evaluated")
	var sent []*block_mock.Example
	runs := 0

	// Force fresh evaluations without changing the World.
	err := world.SendViews(ctx, tb.Engine, func(ctx context.Context, _ world.WorldState) (*block_mock.Example, error) {
		// Stop after evaluating each repeated projection.
		if runs == len(projections) {
			return nil, stop
		}

		// Schedule an immediate rerun while returning the next immutable value.
		next := projections[runs]
		runs++
		world.RerunAt(ctx, time.Now())
		return next, nil
	}, func(next *block_mock.Example) error {
		sent = append(sent, next)
		return nil
	})

	// Send nil once and the changed content once, not their equal repetitions.
	if !errors.Is(err, stop) {
		t.Fatalf("SendViews = %v, want %v", err, stop)
	}
	if len(sent) != 2 {
		t.Fatalf("sent %d projections, want 2", len(sent))
	}
	if sent[0] != nil {
		t.Fatalf("first projection = %v, want nil", sent[0])
	}
	if !sent[1].EqualVT(changed) {
		t.Fatalf("second projection = %v, want %v", sent[1], changed)
	}
}

// TestSendViewsErrors checks that reader and sender failures end the watch.
func TestSendViewsErrors(t *testing.T) {
	// Share a quiet World across the two immediate failure paths.
	tb := world_testbed.MustDefault(t, t.Context())
	stop := errors.New("projection failed")

	// Preserve a reader's error without sending its partial result.
	err := world.SendViews(t.Context(), tb.Engine, func(context.Context, world.WorldState) (*block_mock.Example, error) {
		return nil, stop
	}, func(*block_mock.Example) error {
		t.Error("sent a failed projection")
		return nil
	})
	if !errors.Is(err, stop) {
		t.Fatalf("reader failure = %v, want %v", err, stop)
	}

	// Preserve a sender's error without evaluating again.
	err = world.SendViews(t.Context(), tb.Engine, func(context.Context, world.WorldState) (*block_mock.Example, error) {
		return nil, nil
	}, func(*block_mock.Example) error {
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("sender failure = %v, want %v", err, stop)
	}
}

// TestWatchViewCancellation checks that caller expiry is not a rerun deadline.
func TestWatchViewCancellation(t *testing.T) {
	// Cancel the caller after the first evaluation asks for a future rerun.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tb := world_testbed.MustDefault(t, t.Context())
	runs := 0
	err := world.WatchView(ctx, tb.Engine, func(ctx context.Context, _ world.WorldState) error {
		runs++
		world.RerunAt(ctx, time.Now().Add(time.Hour))
		cancel()
		return nil
	})

	// End at cancellation without another evaluation.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchView = %v, want context.Canceled", err)
	}
	if runs != 1 {
		t.Fatalf("view ran %d times, want 1", runs)
	}
}

// readViewObject reads one key in its own production Engine transaction.
func readViewObject(ctx context.Context, engine world.Engine, key string) (bool, error) {
	// Discard this snapshot before any client opens the next one.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	// Return existence without retaining an object handle.
	return tx.HasObject(ctx, key)
}

// createViewObject publishes one key through the production Engine interface.
func createViewObject(t *testing.T, ctx context.Context, engine world.Engine, key string) {
	// Keep the write transaction and its object handle scoped to this helper.
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Publish the new object before notifying the view.
	obj, err := tx.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestWatchViewSharedReads checks that a view reruns on a change to a finished
// set added with AddViewReads, and only then.
func TestWatchViewSharedReads(t *testing.T) {
	// Compute the shared value's reads on their own tracked state.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	shared := world.NewReadSet()
	if _, err := shared.Track(tb.WorldState).HasObject(ctx, "view/shared"); err != nil {
		t.Fatal(err)
	}

	// The view reads nothing itself and reruns only for the shared set.
	runs := make(chan bool, 4)
	errCh := make(chan error, 1)
	go func() {
		errCh <- world.WatchView(ctx, tb.Engine, func(ctx context.Context, ws world.WorldState) error {
			world.AddViewReads(ctx, shared, time.Time{})
			found, err := ws.HasObject(ctx, "view/shared")
			runs <- found
			return err
		})
	}()
	if found := waitViewRun(t, ctx, runs); found {
		t.Fatal("first run saw the shared object")
	}

	// An unrelated commit must not rerun the view.
	createViewObject(t, ctx, tb.Engine, "view/unrelated")
	select {
	case <-runs:
		t.Fatal("view reran for a change outside the shared set")
	case <-time.After(100 * time.Millisecond):
	}

	// A commit to the shared set reruns it.
	createViewObject(t, ctx, tb.Engine, "view/shared")
	if found := waitViewRun(t, ctx, runs); !found {
		t.Fatal("rerun did not see the shared object")
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchView = %v, want context.Canceled", err)
	}
}

// TestWatchViewAfterSeqno checks that a starting seqno skips the first run.
func TestWatchViewAfterSeqno(t *testing.T) {
	// Start the view at the seqno the caller already holds.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)

	// Advance the World past zero and take that seqno.
	createViewObject(t, ctx, tb.Engine, "view/held")
	held, err := tb.Engine.GetSeqno(ctx)
	if err != nil || held == 0 {
		t.Fatalf("seqno = %d, %v", held, err)
	}

	// Run the view from the held seqno, reporting where each run reads.
	runs := make(chan uint64, 4)
	errCh := make(chan error, 1)
	go func() {
		errCh <- world.WatchViewWith(ctx, tb.Engine, world.WatchViewOptions{AfterSeqno: held}, func(ctx context.Context, ws world.WorldState) error {
			seqno, err := ws.GetSeqno(ctx)
			runs <- seqno
			return err
		})
	}()

	// No run happens while the World stays at the held seqno.
	select {
	case <-runs:
		t.Fatal("view ran at the held seqno")
	case <-time.After(100 * time.Millisecond):
	}

	// The next change runs it, at a later seqno.
	createViewObject(t, ctx, tb.Engine, "view/next")
	select {
	case seqno := <-runs:
		if seqno <= held {
			t.Fatalf("first run at seqno %d, want above %d", seqno, held)
		}
	case <-ctx.Done():
		t.Fatal("view did not run after the next change")
	}
	cancel()
	<-errCh
}

// TestWatchViewMinInterval checks that a burst of touching commits reruns the
// view once, and that the rerun reads the newest head.
func TestWatchViewMinInterval(t *testing.T) {
	// Watch two keys with a long interval.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	const burst = 5
	last := fmt.Sprintf("view/burst-%d", burst-1)

	// Report whether each run sees the last object.
	opts := world.WatchViewOptions{MinInterval: 300 * time.Millisecond}
	runs := make(chan bool, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- world.WatchViewWith(ctx, tb.Engine, opts, func(ctx context.Context, ws world.WorldState) error {
			found, err := ws.HasObject(ctx, last)
			if err == nil {
				_, err = ws.HasObject(ctx, "view/burst-0")
			}
			runs <- found
			return err
		})
	}()
	if found := waitViewRun(t, ctx, runs); found {
		t.Fatal("first run saw the last object")
	}

	// Commit the burst well inside the interval.
	for i := range burst {
		createViewObject(t, ctx, tb.Engine, fmt.Sprintf("view/burst-%d", i))
	}

	// One rerun sees the newest head, and no second one follows.
	if found := waitViewRun(t, ctx, runs); !found {
		t.Fatal("rerun did not read the newest head")
	}
	select {
	case <-runs:
		t.Fatal("burst reran the view more than once")
	case <-time.After(500 * time.Millisecond):
	}
	cancel()
	<-errCh
}

// waitViewRun returns the next evaluation result, or fails at the deadline.
func waitViewRun[T any](t *testing.T, ctx context.Context, runs <-chan T) T {
	t.Helper()
	select {
	case v := <-runs:
		return v
	case <-ctx.Done():
		t.Fatal("view did not run")
		panic("unreachable")
	}
}

// TestWithViewReads checks that a child evaluation records its transactions,
// nested shared sets and deadline into its own set, and that a view reruns
// for a nested read once the caller adds the finished set.
func TestWithViewReads(t *testing.T) {
	// A nested shared value read view/nested.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	engine := world.NewReadTrackingEngine(tb.Engine)
	nested := world.NewReadSet()
	if _, err := nested.Track(tb.WorldState).HasObject(ctx, "view/nested"); err != nil {
		t.Fatal(err)
	}

	// The view computes a value in a child that reads view/inner and reuses
	// the nested value, then adds the finished set.
	deadline := time.Now().Add(time.Hour)
	runs := make(chan bool, 4)
	errCh := make(chan error, 1)
	go func() {
		errCh <- world.WatchView(ctx, tb.Engine, func(ctx context.Context, ws world.WorldState) error {
			// Share the child's finished reads with the view.
			set, err := computeShared(ctx, engine, nested, deadline)
			if err != nil {
				return err
			}
			world.AddViewReads(ctx, set, time.Time{})

			// Report whether the nested object exists yet.
			found, err := ws.HasObject(ctx, "view/nested")
			runs <- found
			return err
		})
	}()
	if found := waitViewRun(t, ctx, runs); found {
		t.Fatal("first run saw the nested object")
	}

	// A commit to the nested read reruns the view.
	createViewObject(t, ctx, tb.Engine, "view/nested")
	if found := waitViewRun(t, ctx, runs); !found {
		t.Fatal("rerun did not see the nested object")
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchView = %v, want context.Canceled", err)
	}
}

// computeShared computes a shared value in a child evaluation that reads
// view/inner, reuses nested and asks to rerun at deadline, and checks that the
// finished set holds all three.
func computeShared(ctx context.Context, engine world.Engine, nested *world.ReadSet, deadline time.Time) (*world.ReadSet, error) {
	// Read view/inner through a tracked transaction in the child.
	set := world.NewReadSet()
	child, finish := world.WithViewReads(ctx, set)
	tx, err := engine.NewTransaction(child, false)
	if err != nil {
		return nil, err
	}
	_, err = tx.HasObject(child, "view/inner")
	tx.Discard()
	if err != nil {
		return nil, err
	}

	// Reuse the nested value and ask for a rerun.
	world.AddViewReads(child, nested, time.Time{})
	world.RerunAt(child, deadline)
	if got := finish(); !got.Equal(deadline) {
		return nil, fmt.Errorf("child deadline %v, want %v", got, deadline)
	}

	// The finished set holds both reads.
	for _, key := range []string{"view/inner", "view/nested"} {
		if !set.Touched(&world.ChangeSet{Keys: []string{key}}) {
			return nil, fmt.Errorf("child set missed %s", key)
		}
	}
	return set, nil
}
