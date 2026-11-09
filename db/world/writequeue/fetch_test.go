package writequeue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/testbed"
)

// TestQueuePeerFetch checks that a body's read only a peer can serve is
// fetched with the writer lock released: a write of another actor queued
// behind it commits during the fetch, a later write of the same actor commits
// after it, and the body sees the fetched block when it runs again.
func TestQueuePeerFetch(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithChangelog())
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	q := New(tb.Engine, Options{})

	data := []byte("a block only a peer holds")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	const fetchTime = 2 * time.Second

	var mtx sync.Mutex
	var order []string
	record := func(name string) {
		mtx.Lock()
		order = append(order, name)
		mtx.Unlock()
	}
	create := func(ctx context.Context, tx world.Tx, key string) error {
		object, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(object)
		return err
	}
	type result struct {
		err error
		at  time.Time
	}
	launch := func(actor uint64, body func(context.Context, world.Tx) error) <-chan result {
		done := make(chan result, 1)
		go func() {
			_, err := q.Write(ctx, actor, nil, body, nil)
			done <- result{err, time.Now()}
		}()
		return done
	}

	// The first write reads the peer's block; its first attempt defers it.
	var attempts int
	fetched := make(chan struct{})
	start := time.Now()
	first := launch(1, func(ctx context.Context, tx world.Tx) error {
		attempts++
		reads := block.GetPeerReads(ctx)
		if reads == nil {
			return errors.New("body ran without deferred peer reads")
		}
		stored := reads.Lookup(ref)
		if stored == nil {
			reads.Defer(ref, func(ctx context.Context) (*block.StoredBlock, error) {
				defer close(fetched)
				select {
				case <-time.After(fetchTime):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return &block.StoredBlock{Data: data, RefsKnown: true}, nil
			})
			// A body may take the failed read for an absent block and go on.
			return create(ctx, tx, "first-without-block")
		}
		if string(stored.Data) != string(data) {
			return errors.New("fetched block differs")
		}
		record("first")
		return create(ctx, tx, "first")
	})

	// Wait until the first write is parked, then queue the others.
	deadline := time.Now().Add(10 * time.Second)
	for {
		q.mtx.Lock()
		parked := len(q.parked)
		q.mtx.Unlock()
		if parked == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first write never parked")
		}
		time.Sleep(time.Millisecond)
	}
	same := launch(1, func(ctx context.Context, tx world.Tx) error {
		record("same-actor")
		return create(ctx, tx, "same-actor")
	})
	other := launch(2, func(ctx context.Context, tx world.Tx) error {
		record("other-actor")
		return create(ctx, tx, "other-actor")
	})

	// The other actor's write commits while the fetch runs.
	got := <-other
	if got.err != nil {
		t.Fatal(got.err)
	}
	if d := got.at.Sub(start); d >= fetchTime {
		t.Fatalf("other actor's write waited %v for the fetch", d)
	}
	for _, done := range []<-chan result{first, same} {
		if got := <-done; got.err != nil {
			t.Fatal(got.err)
		}
	}
	<-fetched

	mtx.Lock()
	defer mtx.Unlock()
	want := []string{"other-actor", "first", "same-actor"}
	if len(order) != len(want) {
		t.Fatalf("order %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v", order, want)
		}
	}
	if attempts != 2 {
		t.Fatalf("first write ran %d times, want 2", attempts)
	}

	// The first attempt's change was discarded with its transaction.
	tx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if exists, err := tx.HasObject(ctx, "first-without-block"); err != nil || exists {
		t.Fatal("the deferred attempt's change committed", exists, err)
	}
	for _, key := range []string{"first", "same-actor", "other-actor"} {
		if exists, err := tx.HasObject(ctx, key); err != nil || !exists {
			t.Fatal(key, exists, err)
		}
	}
}
