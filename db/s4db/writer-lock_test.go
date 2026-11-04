package s4db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestWriterLockOrder walks a handle's writer lock through a queue: a
// checkpoint step goes before a writer that arrived earlier, the waiting
// check sees both, and the lock is free after the last release.
func TestWriterLockOrder(t *testing.T) {
	// Hold the lock, without the background compactor.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "lock.s4wave"), Options{SingleProcess: true})
	defer db.Close()
	if wait, _ := db.compactor.SetRoutine(nil); wait != nil {
		<-wait
	}
	w := db.w
	if err := w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, _ := w.tryLock(); ok {
		t.Fatal("tryLock took a held lock")
	}
	if db.WritersWaiting() {
		t.Fatal("a writer waits on an uncontended lock")
	}

	// Queue a writer, then a checkpoint step.
	order := make(chan string, 2)
	wait := func(name string, first bool, prio int) {
		go func() {
			if err := w.lockAs(ctx, first); err != nil {
				t.Error(err)
				return
			}
			order <- name
		}()
		for !w.mtx.Waiting(prio) {
			time.Sleep(time.Millisecond)
		}
	}
	wait("writer", false, prioWriter)
	if !db.WritersWaiting() {
		t.Fatal("the waiting writer is not reported")
	}
	wait("checkpoint", true, prioCheckpoint)

	// Each release grants exactly the next waiter.
	for _, want := range []string{"checkpoint", "writer"} {
		w.unlock(nil)
		if got := <-order; got != want {
			t.Fatalf("granted %q, want %q", got, want)
		}
	}
	if db.WritersWaiting() {
		t.Fatal("a writer is reported after the queue drained")
	}
	w.unlock(nil)
	ok, err := w.tryLock()
	if err != nil || !ok {
		t.Fatalf("the lock stayed held after the last release: %v", err)
	}
	w.unlock(nil)
}
