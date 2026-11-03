package feed

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/s4wave/spacewave/db/world/viewstream"

	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/testbed"
)

// TestFeed follows real World revisions through shared reads and frames.
func TestFeed(t *testing.T) {
	// Start a real World and track reads made through its engine.
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithChangelog())
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	engine := world.NewReadTrackingEngine(tb.Engine)
	options := Options[world.Engine]{
		Context:  func(context.Context) context.Context { return context.Background() },
		Public:   func(e world.Engine) world.Engine { return e },
		Admit:    func(context.Context, world.Engine) error { return nil },
		Revision: func(ctx context.Context, e world.Engine) (uint64, error) { return e.GetSeqno(ctx) },
		Own:      func(world.Engine) func(uint64) (bool, <-chan struct{}) { return nil },
		Watch: func(ctx context.Context, e world.Engine, _ *viewstream.Pokes, read func(context.Context) error) error {
			return world.WatchViewWith(ctx, e, world.WatchViewOptions{MinInterval: time.Nanosecond}, func(ctx context.Context, _ world.WorldState) error { return read(ctx) })
		},
	}
	hub := NewHub(time.Duration(0), 3, options)
	var reads, encodes atomic.Int64
	feed := &Feed[world.Engine, *resource.ViewVersion]{
		Read: func(ctx context.Context, e world.Engine, key string) (*resource.ViewVersion, error) {
			// Read one indexed object in the production snapshot.
			reads.Add(1)
			tx, err := e.NewTransaction(ctx, false)
			if err != nil {
				return nil, err
			}
			defer tx.Discard()
			exists, err := tx.HasObject(ctx, key)
			if err != nil {
				return nil, err
			}
			value := "missing"
			if exists {
				value = "present"
			}
			return &resource.ViewVersion{Version: value}, nil
		},
		Equal:  func(a, b *resource.ViewVersion) bool { return a.EqualVT(b) },
		Encode: func(v *resource.ViewVersion) ([]byte, error) { encodes.Add(1); return v.MarshalVT() },
	}

	// Join two subscribers and keep one delivery blocked.
	watch := func(block <-chan struct{}) (<-chan *Publication[*resource.ViewVersion], context.CancelFunc, <-chan error) {
		child, cancel := context.WithCancel(ctx)
		pubs := make(chan *Publication[*resource.ViewVersion], 4)
		done := make(chan error, 1)
		go func() {
			done <- WatchFeed(child, hub, world.Engine(engine), feed, "item", func(pub *Publication[*resource.ViewVersion]) error {
				select {
				case pubs <- pub:
				case <-child.Done():
					return child.Err()
				}
				if block != nil {
					select {
					case <-block:
					case <-child.Done():
						return child.Err()
					}
				}
				return nil
			})
		}()
		return pubs, cancel, done
	}
	hold := make(chan struct{})
	slow, stopSlow, slowDone := watch(hold)
	first := <-slow
	fast, stopFast, fastDone := watch(nil)
	if again := <-fast; again != first || reads.Load() != 1 || encodes.Load() != 1 {
		t.Fatal("subscribers did not share one read and encoding")
	}

	// A real commit advances the fast subscriber while the slow one is blocked.
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	object, err := tx.CreateObject(ctx, "item", nil)
	if err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	world.ReleaseObjectState(object)
	err = tx.Commit(ctx)
	tx.Discard()
	if err != nil {
		t.Fatal(err)
	}
	changed := <-fast
	if changed.Value.Version != "present" || changed.Version() == first.Version() {
		t.Fatal("changed object did not publish")
	}
	if delta := changed.Since(first.Version()); delta.Full || len(delta.Changed) != 1 {
		t.Fatal("retained base did not produce changes", delta)
	}
	if _, err := changed.Frame(changed.Version()); err != nil {
		t.Fatal(err)
	}
	if _, err := changed.Frame(""); err != nil {
		t.Fatal(err)
	}
	if encodes.Load() != 2 {
		t.Fatal("frames reencoded the publication")
	}

	// Late subscribers get the current publication; cancellation frees each watch.
	late, stopLate, lateDone := watch(nil)
	if pub := <-late; pub != changed {
		t.Fatal("late join used an older publication")
	}
	close(hold)
	if pub := <-slow; pub != changed {
		t.Fatal("slow subscriber did not catch up")
	}
	for _, stop := range []context.CancelFunc{stopSlow, stopFast, stopLate} {
		stop()
	}
	for _, done := range []<-chan error{slowDone, fastDone, lateDone} {
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
}

// TestFeedLinger keeps a source until its final subscriber's timer expires.
func TestFeedLinger(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Start one source that waits until the hub cancels it.
		started, ended := make(chan struct{}), make(chan struct{})
		options := Options[int]{
			Context: func(context.Context) context.Context { return context.Background() },
			Public:  func(i int) int { return i },
			Admit:   func(context.Context, int) error { return nil },
			Own:     func(int) func(uint64) (bool, <-chan struct{}) { return nil },
			Watch: func(ctx context.Context, _ int, _ *viewstream.Pokes, _ func(context.Context) error) error {
				close(started)
				<-ctx.Done()
				close(ended)
				return ctx.Err()
			},
		}
		hub := NewHub(time.Minute, 2, options)
		feed := &Feed[int, int]{}
		run, leave := joinFeed(t.Context(), hub, 0, feed, "key")
		<-started
		leave()
		time.Sleep(30 * time.Second)
		joined, leaveAgain := joinFeed(t.Context(), hub, 0, feed, "key")
		if joined != run {
			t.Fatal("rejoin replaced a lingering source")
		}
		time.Sleep(40 * time.Second)
		select {
		case <-ended:
			t.Fatal("old linger timer stopped rejoined source")
		default:
		}

		// The current timer owns final cancellation and removes its run.
		leaveAgain()
		time.Sleep(time.Minute)
		<-ended
		synctest.Wait()
		hub.mtx.Lock()
		defer hub.mtx.Unlock()
		if len(hub.runs) != 0 {
			t.Fatal("expired source remained in hub")
		}
	})
}
