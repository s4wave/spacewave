package s4db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestWriterLockOrder walks a handle's writer lock through a queue: a
// checkpoint step goes before a writer that arrived earlier, a background
// writer goes after both, the waiting check sees only the foreground ones,
// and the lock is free after the last release.
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

	// Queue a background writer, a writer, then a checkpoint step.
	order := make(chan string, 3)
	wait := func(name string, wctx context.Context, prio int) {
		go func() {
			lock := w.lock
			if prio == prioCheckpoint {
				lock = w.lockFirst
			}
			if err := lock(wctx); err != nil {
				t.Error(err)
				return
			}
			order <- name
		}()
		for !w.mtx.Waiting(prio) {
			time.Sleep(time.Millisecond)
		}
	}
	wait("background", kvtx.WithBackgroundWriteContext(ctx), prioBackground)
	if db.WritersWaiting() {
		t.Fatal("a waiting background writer is reported as a writer")
	}
	wait("writer", ctx, prioWriter)
	if !db.WritersWaiting() {
		t.Fatal("the waiting writer is not reported")
	}
	wait("checkpoint", ctx, prioCheckpoint)

	// Each release grants exactly the next waiter.
	for _, want := range []string{"checkpoint", "writer", "background"} {
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
