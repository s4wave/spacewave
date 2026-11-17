//go:build darwin || linux || windows

package s4db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// admissionCall holds a controlled native call until release or cancellation.
type admissionCall struct {
	// release lets the admitted body complete.
	release chan struct{}
	// cancel ends both admission and the body during failure cleanup.
	cancel context.CancelFunc
	// done closes after the call releases its transaction or scheduler grant.
	done chan struct{}
	// err is the result, read only after done closes.
	err error
}

// openAdmissionTest opens the native writer with automatic maintenance stopped.
func openAdmissionTest(t *testing.T) (context.Context, *DB) {
	// Close the file after every controlled call's cleanup has joined it.
	t.Helper()
	db := openTest(t, filepath.Join(t.TempDir(), "admission.s4wave"), Options{})
	t.Cleanup(func() { _ = db.Close() })
	db.sched.stop()

	// Bound failed rendezvous without using time to establish queue ordering.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx, db
}

// startAdmissionCall starts a transaction, optimistic commit or scheduler slice.
// Every body and admission wait is cancellable, including failure cleanup.
func startAdmissionCall(t *testing.T, ctx context.Context, db *DB, order chan<- string, name, path string) *admissionCall {
	// Register cleanup before launching a call that may hold the native writer.
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	call := &admissionCall{release: make(chan struct{}), cancel: cancel, done: make(chan struct{})}
	t.Cleanup(func() {
		// Release held bodies before Close waits for their transactions.
		cancel()
		<-call.done
	})

	// Report acquisition from inside the actual admitted transaction body.
	run := func() error {
		// A buffered order channel still has cancellation if a test exits early.
		select {
		case order <- name:
		case <-ctx.Done():
			return ctx.Err()
		}

		// A failed assertion must not leave a body blocking DB.Close.
		select {
		case <-call.release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	go func() {
		// Publish the result before notifying normal checks and cleanup.
		defer close(call.done)
		switch path {
		case "slice":
			call.err = db.w.locked(ctx, run)
		case "optimistic":
			call.err = kvtx.RunOptimisticTransaction(ctx, func(openCtx context.Context) (kvtx.Tx, error) {
				return db.NewTransaction(openCtx, true)
			}, func(bodyCtx context.Context, tx kvtx.Tx) error {
				return kvtx.AtCommit(bodyCtx, tx, func(context.Context, kvtx.Tx) error { return run() })
			}, kvtx.RetryInvalidSnapshot)
		default:
			// The ordinary path holds the writer until Discard.
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				call.err = err
				return
			}
			defer tx.Discard()
			call.err = run()
		}
	}()
	return call
}

// waitAdmissionCall joins a completed call and checks its expected result.
func waitAdmissionCall(t *testing.T, ctx context.Context, call *admissionCall, want error) {
	// The deadline only bounds a failed test; done is the completion signal.
	t.Helper()
	select {
	case <-call.done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !errors.Is(call.err, want) {
		t.Fatalf("admitted call returned %v, want %v", call.err, want)
	}
}

// waitAdmissionGrant checks the next body to acquire the real native writer.
func waitAdmissionGrant(t *testing.T, ctx context.Context, order <-chan string, want string) {
	// Accept only an observed body entry, never a delay without one.
	t.Helper()
	select {
	case got := <-order:
		if got != want {
			t.Fatalf("writer grant %s, want %s", got, want)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

// waitAdmissionQueue observes registration under the coordinator's actual mutex.
// The timer yields between observations; elapsed time never proves registration.
func waitAdmissionQueue(t *testing.T, ctx context.Context, a *writerAdmission, foreground, background int) {
	// Count only test queues; production enqueue, grant and cancellation never scan.
	t.Helper()
	count := func(q writerRequests) int {
		n := 0
		for r := q.head; r != nil; r = r.next {
			n++
		}
		return n
	}
	for {
		// Read both queues within the same mutex hold as their mutations.
		a.mtx.Lock()
		ready := count(a.foreground) == foreground && count(a.background) == background
		a.mtx.Unlock()
		if ready {
			return
		}

		// Bound a missing registration without assuming goroutine start order.
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
}

// TestWriterAdmissionRoundFour reproduces the two independently aged callers.
// It also counts an already-running slice against later foreground arrivals.
func TestWriterAdmissionRoundFour(t *testing.T) {
	// Exercise both transaction-open and real optimistic Commit admission.
	t.Setenv(kvtx.OptimisticEnv, "on")
	for _, path := range []string{"transaction", "optimistic"} {
		t.Run(path, func(t *testing.T) {
			// Start with genuinely running background work, outside any cohort.
			ctx, db := openAdmissionTest(t)
			order := make(chan string, 8)
			w := startAdmissionCall(t, ctx, db, order, "W", "slice")
			waitAdmissionGrant(t, ctx, order, "W")

			// Queue two background callers before the first foreground request.
			b1 := startAdmissionCall(t, block.WithBackground(ctx), db, order, "B1", path)
			waitAdmissionQueue(t, ctx, &db.w.admission, 0, 1)
			b2 := startAdmissionCall(t, ctx, db, order, "B2", "slice")
			waitAdmissionQueue(t, ctx, &db.w.admission, 0, 2)
			f1 := startAdmissionCall(t, ctx, db, order, "F1", path)
			waitAdmissionQueue(t, ctx, &db.w.admission, 1, 2)
			close(w.release)
			waitAdmissionGrant(t, ctx, order, "F1")

			// Only one background turn may precede the next foreground cohort.
			f2 := startAdmissionCall(t, ctx, db, order, "F2", "transaction")
			waitAdmissionQueue(t, ctx, &db.w.admission, 1, 2)
			close(f1.release)
			waitAdmissionGrant(t, ctx, order, "B1")

			// An arrival during B1 must also precede B2: B1 counts as its slice.
			f3 := startAdmissionCall(t, ctx, db, order, "F3", "transaction")
			waitAdmissionQueue(t, ctx, &db.w.admission, 2, 1)
			close(b1.release)
			waitAdmissionGrant(t, ctx, order, "F2")
			close(f2.release)
			waitAdmissionGrant(t, ctx, order, "F3")
			close(f3.release)
			waitAdmissionGrant(t, ctx, order, "B2")
			close(b2.release)
			for _, call := range []*admissionCall{w, f1, b1, f2, f3, b2} {
				waitAdmissionCall(t, ctx, call, nil)
			}
		})
	}
}

// TestWriterAdmissionSteady keeps overlapping foreground requests registered.
// Background must progress when the finite captured cohort ends.
func TestWriterAdmissionSteady(t *testing.T) {
	// Hold a background slice while two foreground calls form the first cohort.
	ctx, db := openAdmissionTest(t)
	order := make(chan string, 8)
	w := startAdmissionCall(t, ctx, db, order, "W", "slice")
	waitAdmissionGrant(t, ctx, order, "W")
	f1 := startAdmissionCall(t, ctx, db, order, "F1", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 1, 0)
	f2 := startAdmissionCall(t, ctx, db, order, "F2", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 2, 0)
	b := startAdmissionCall(t, ctx, db, order, "B", "slice")
	waitAdmissionQueue(t, ctx, &db.w.admission, 2, 1)
	close(w.release)
	waitAdmissionGrant(t, ctx, order, "F1")

	// Refill the foreground queue before each current cohort member releases.
	f3 := startAdmissionCall(t, ctx, db, order, "F3", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 2, 1)
	close(f1.release)
	waitAdmissionGrant(t, ctx, order, "F2")
	f4 := startAdmissionCall(t, ctx, db, order, "F4", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 2, 1)
	close(f2.release)
	waitAdmissionGrant(t, ctx, order, "B")
	if !db.WritersWaiting() {
		t.Fatal("steady foreground demand disappeared before background grant")
	}

	// Later foreground calls finish after the shared background turn.
	close(b.release)
	waitAdmissionGrant(t, ctx, order, "F3")
	close(f3.release)
	waitAdmissionGrant(t, ctx, order, "F4")
	close(f4.release)
	for _, call := range []*admissionCall{w, f1, f2, f3, f4, b} {
		waitAdmissionCall(t, ctx, call, nil)
	}
}

// TestWriterAdmissionCancelTail cancels consecutive cohort tails with a later
// arrival waiting, without extending the cohort or losing its background turn.
func TestWriterAdmissionCancelTail(t *testing.T) {
	// Capture three foreground members behind an already-running slice.
	ctx, db := openAdmissionTest(t)
	order := make(chan string, 8)
	w := startAdmissionCall(t, ctx, db, order, "W", "slice")
	waitAdmissionGrant(t, ctx, order, "W")
	f1 := startAdmissionCall(t, ctx, db, order, "F1", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 1, 0)
	f2 := startAdmissionCall(t, ctx, db, order, "F2", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 2, 0)
	f3 := startAdmissionCall(t, ctx, db, order, "F3", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 3, 0)
	b1 := startAdmissionCall(t, ctx, db, order, "B1", "slice")
	waitAdmissionQueue(t, ctx, &db.w.admission, 3, 1)
	b2 := startAdmissionCall(t, ctx, db, order, "B2", "slice")
	waitAdmissionQueue(t, ctx, &db.w.admission, 3, 2)
	close(w.release)
	waitAdmissionGrant(t, ctx, order, "F1")

	// Cancellation moves the tail through F2 to an empty remaining cohort.
	f4 := startAdmissionCall(t, ctx, db, order, "F4", "transaction")
	waitAdmissionQueue(t, ctx, &db.w.admission, 3, 2)
	f3.cancel()
	waitAdmissionCall(t, ctx, f3, context.Canceled)
	f2.cancel()
	waitAdmissionCall(t, ctx, f2, context.Canceled)
	waitAdmissionQueue(t, ctx, &db.w.admission, 1, 2)
	close(f1.release)
	waitAdmissionGrant(t, ctx, order, "B1")

	// Neither tail cancellation nor B1's turn may move B2 ahead of F4.
	close(b1.release)
	waitAdmissionGrant(t, ctx, order, "F4")
	close(f4.release)
	waitAdmissionGrant(t, ctx, order, "B2")
	close(b2.release)
	for _, call := range []*admissionCall{w, f1, f4, b1, b2} {
		waitAdmissionCall(t, ctx, call, nil)
	}
}

// TestWriterAdmissionCancelGrant forces cancellation between grant and wakeup.
func TestWriterAdmissionCancelGrant(t *testing.T) {
	// Hold the coordinator while a cancellable foreground request registers.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var a writerAdmission
	if err := a.lock(ctx, true); err != nil {
		t.Fatal(err)
	}
	request, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() {
		// A wrongly successful grant still releases ownership on test failure.
		err := a.lock(request, false)
		if err == nil {
			a.unlock()
		}
		done <- err
	}()
	waitAdmissionQueue(t, ctx, &a, 1, 0)

	// Freeze the granted caller at mtx until its context has been cancelled.
	a.mtx.Lock()
	a.holder = nil
	a.dispatch()
	stop()
	a.mtx.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled grant returned %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Cancellation must return the grant and foreground waiting count once.
	if a.waiting.Load() != 0 {
		t.Fatal("cancelled grant retained foreground demand")
	}
	if !a.tryLock() {
		t.Fatal("cancelled grant retained writer ownership")
	}
	a.unlock()
}

// TestWriterAdmissionLockFailure returns grants after native lock I/O failure.
func TestWriterAdmissionLockFailure(t *testing.T) {
	// A closed real descriptor makes native lock acquisition fail after grant.
	f, err := os.Create(filepath.Join(t.TempDir(), "closed.s4wave"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	w := writer{db: &DB{s: &osFile{File: f}}}
	if err := w.take(t.Context(), true); err == nil {
		t.Fatal("writer acquired a closed file")
	}
	if !w.admission.tryLock() {
		t.Fatal("failed native lock retained the writer grant")
	}
	w.admission.unlock()

	// Optional closing marks must return their grant on the same failure.
	if ok, err := w.tryLock(t.Context()); ok || err == nil {
		t.Fatalf("optional native lock: %v, %v", ok, err)
	}
	if !w.admission.tryLock() {
		t.Fatal("failed optional native lock retained the writer grant")
	}
	w.admission.unlock()
}
