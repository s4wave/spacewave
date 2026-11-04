package s4db

import (
	"context"
	"testing"
	"time"
)

// TestWriterLockOrder walks the lock through a queue: a checkpoint step goes
// before writers that arrived earlier, writers follow in arrival order, a
// canceled waiter leaves the queue, and a grant that races a cancel reaches
// the next waiter instead of being lost.
func TestWriterLockOrder(t *testing.T) {
	ctx := context.Background()
	var l writerLock
	if err := l.lock(ctx, false); err != nil {
		t.Fatal(err)
	}
	if l.tryLock() {
		t.Fatal("tryLock took a held lock")
	}

	// Queue two writers, one that cancels, then a checkpoint step.
	order := make(chan string, 3)
	wait := func(name string, first bool, queued int) {
		go func() {
			if err := l.lock(ctx, first); err != nil {
				t.Error(err)
				return
			}
			order <- name
		}()
		for {
			l.mtx.Lock()
			n := len(l.first) + len(l.rest)
			l.mtx.Unlock()
			if n == queued {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}
	wait("writer 1", false, 1)
	canceled, cancel := context.WithCancel(ctx)
	gone := make(chan error, 1)
	go func() { gone <- l.lock(canceled, false) }()
	for {
		l.mtx.Lock()
		n := len(l.rest)
		l.mtx.Unlock()
		if n == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	wait("writer 2", false, 3)
	wait("checkpoint", true, 4)
	cancel()
	if err := <-gone; err == nil {
		t.Fatal("a canceled waiter took the lock")
	}

	// Each release grants exactly the next waiter.
	for _, want := range []string{"checkpoint", "writer 1", "writer 2"} {
		l.unlock()
		if got := <-order; got != want {
			t.Fatalf("granted %q, want %q", got, want)
		}
	}

	// A grant that arrives with the cancel passes on. Grant the waiter while
	// ctx is already done, so its select may take either case.
	for range 200 {
		canceled, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- l.lock(canceled, false) }()
		go cancel()
		l.unlock()
		if err := <-done; err == nil {
			l.unlock()
		}
		if err := l.lock(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	l.unlock()
	if !l.tryLock() {
		t.Fatal("the lock stayed held after the last release")
	}
}
