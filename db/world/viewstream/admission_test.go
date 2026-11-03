package viewstream

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

// TestAdmission shares bounded first reads in priority order and releases once.
func TestAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Occupy the only slot, then queue two priorities and an abandoned caller.
		a := NewAdmission(1, 3, 2, 0, nil)
		release, err := a.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		order := make(chan int, 3)
		queue := func(ctx context.Context, p int) {
			go func() {
				done, err := a.Acquire(WithPriority(ctx, p))
				if err != nil {
					order <- -1
					return
				}
				order <- p
				done()
			}()
		}
		queue(t.Context(), 2)
		queue(t.Context(), 0)
		cancelled, cancel := context.WithCancel(t.Context())
		queue(cancelled, 1)
		synctest.Wait()
		cancel()
		synctest.Wait()
		if got := <-order; got != -1 {
			t.Fatal("cancelled caller acquired", got)
		}
		release()
		release()
		synctest.Wait()
		if high, low := <-order, <-order; high != 0 || low != 2 {
			t.Fatal("priority order", high, low)
		}
		if a.free != 1 {
			t.Fatal("slot released more than once", a.free)
		}

		// An explicit maximum hold returns a stalled slot; its later release is inert.
		a = NewAdmission(1, 1, 0, time.Minute, nil)
		late, err := a.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if a.free != 1 {
			t.Fatal("maximum hold did not free slot")
		}
		late()
		if a.free != 1 {
			t.Fatal("late release freed an extra slot")
		}
	})
}

// TestPacer lets own writes and pokes bypass the interval without polling.
func TestPacer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Start an interval and wait for the owning actor's next write.
		writes, pokes := NewWrites(), NewPokes()
		pacer := NewPacer(nil, writes, 7, pokes, time.Hour)
		if err := pacer.Started(t.Context()); err != nil {
			t.Fatal(err)
		}
		pacer.watched(1)
		done := make(chan error, 1)
		go func() { done <- pacer.pace(t.Context()) }()
		synctest.Wait()
		writes.Record([]uint64{8}, 2)
		synctest.Wait()
		if len(done) != 0 {
			t.Fatal("another actor's write bypassed pacing")
		}
		writes.Record([]uint64{7}, 2)
		if err := <-done; err != nil {
			t.Fatal(err)
		}

		// A poke also passes immediately, and cancellation ends the next wait.
		if err := pacer.Started(t.Context()); err != nil {
			t.Fatal(err)
		}
		pacer.watched(2)
		go func() { done <- pacer.pace(t.Context()) }()
		synctest.Wait()
		pokes.Poke()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := pacer.Started(t.Context()); err != nil {
			t.Fatal(err)
		}
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		if err := pacer.pace(cancelled); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}
