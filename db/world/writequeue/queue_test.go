package writequeue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/viewstream"
	"github.com/s4wave/spacewave/testbed"
)

// TestQueue preserves queued order, caller cancellation and durable visibility.
func TestQueue(t *testing.T) {
	// Open the real World and record the durable actors of each batch.
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithChangelog())
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	owners := viewstream.NewWrites()
	q := New(tb.Engine, Options{Owners: owners})
	type result struct {
		revision uint64
		err      error
	}
	launch := func(ctx context.Context, actor uint64, body func(context.Context, world.Tx) error) <-chan result {
		done := make(chan result, 1)
		go func() { revision, err := q.Write(ctx, actor, nil, body, nil); done <- result{revision, err} }()
		return done
	}
	create := func(ctx context.Context, tx world.Tx, key string) error {
		object, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(object)
		return err
	}

	// Hold the first body so later writes queue behind its transaction.
	started, release := make(chan struct{}), make(chan struct{})
	first := launch(ctx, 1, func(ctx context.Context, tx world.Tx) error {
		close(started)
		<-release
		return create(ctx, tx, "first")
	})
	<-started
	var abandoned atomic.Bool
	leaving, cancel := context.WithCancel(ctx)
	left := launch(leaving, 2, func(context.Context, world.Tx) error { abandoned.Store(true); return nil })
	second := launch(ctx, 3, func(ctx context.Context, tx world.Tx) error {
		exists, err := tx.HasObject(ctx, "first")
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("later write missed its predecessor")
		}
		return create(ctx, tx, "second")
	})
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for q.Status().Pending < 2 {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("writers never queued")
		}
	}
	if q.OldestWait(time.Now()) <= 0 {
		t.Fatal("pending writes had no wait")
	}
	cancel()
	if answer := <-left; !errors.Is(answer.err, context.Canceled) {
		t.Fatal(answer)
	}
	close(release)

	// Successful answers reflect durable state, and the abandoned body never runs.
	for _, done := range []<-chan result{first, second} {
		answer := <-done
		if answer.err != nil || answer.revision == 0 {
			t.Fatal(answer)
		}
	}
	if abandoned.Load() {
		t.Fatal("abandoned writer ran later")
	}
	tx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, key := range []string{"first", "second"} {
		if exists, err := tx.HasObject(ctx, key); err != nil || !exists {
			t.Fatal(key, exists, err)
		}
	}
	if owners.Latest(1) == 0 || owners.Latest(3) == 0 || owners.Latest(2) != 0 {
		t.Fatal("durable actor notifications did not match committed writes")
	}
}
