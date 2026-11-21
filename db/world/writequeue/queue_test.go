package writequeue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/coord"
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

// gatedAdmitTx controls when the real World can admit a test transaction.
type gatedAdmitTx struct {
	Transaction
	// beforeAdmit returns a refusal or releases the attempt to admit.
	beforeAdmit func() error
}

// Admit checks the test gate, then admits the World transaction.
func (t *gatedAdmitTx) Admit(ctx context.Context) (world.Admission, error) {
	if err := t.beforeAdmit(); err != nil {
		return nil, err
	}
	return t.Transaction.Admit(ctx)
}

// TestQueueWithholdsErrorReadingBatch checks that a write whose unchanged
// result read an earlier write of its batch answers only from a World where
// that write committed: the engine refuses the first batch, and the reader's
// answer comes from the attempt that commits, not the discarded one.
func TestQueueWithholdsErrorReadingBatch(t *testing.T) {
	// Open a World whose first admission is refused and whose second waits.
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithChangelog())
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	retrying, release := make(chan struct{}), make(chan struct{})
	defer func() {
		if release != nil {
			close(release)
		}
	}()
	gate := release
	var admissions, created, checked int
	unchanged := errors.New("unchanged")
	discarded, durable := errors.New("discarded"), errors.New("durable")
	attemptKey := func() error {
		if admissions == 0 {
			return discarded
		}
		return durable
	}
	q := New(tb.Engine, Options{
		Unchanged: unchanged,
		Wrap: func(ctx context.Context, tx world.Tx) Transaction {
			return &gatedAdmitTx{Transaction: wrapBasic(ctx, tx), beforeAdmit: func() error {
				admissions++
				switch admissions {
				case 1:
					return coord.ErrStaleGeneration
				case 2:
					close(retrying)
					<-gate
				}
				return nil
			}}
		},
	})

	// Queue a write that creates the attempt's key and one that reads it.
	first := &queuedWrite{ctx: ctx, arrival: 1, done: make(chan struct{}), name: func() string { return "create" }}
	first.body = func(ctx context.Context, tx world.Tx) error {
		created++
		object, err := tx.CreateObject(ctx, attemptKey().Error(), nil)
		world.ReleaseObjectState(object)
		return err
	}
	second := &queuedWrite{ctx: ctx, arrival: 2, done: make(chan struct{}), name: func() string { return "check" }}
	second.body = func(ctx context.Context, tx world.Tx) error {
		checked++
		key := attemptKey()
		found, err := tx.HasObject(ctx, key.Error())
		if err != nil {
			return err
		}
		if !found {
			return errors.New("reader missed the write before it")
		}
		return errors.Join(unchanged, key)
	}
	q.pending, q.draining = []*queuedWrite{first, second}, true
	go q.drain()

	// The second admission holds; the reader ran in both batches and has
	// no answer.
	<-retrying
	if created != 2 || checked != 2 {
		t.Fatalf("%d creates and %d checks before the retry admitted, want 2 and 2", created, checked)
	}
	if second.ended() {
		t.Fatalf("the reader answered from an uncommitted batch: %v", second.err)
	}
	close(release)
	release = nil

	// The reader answers from the World holding the committed write.
	<-first.done
	<-second.done
	if first.err != nil || first.seqno == 0 {
		t.Fatalf("first write: revision %d, error %v", first.seqno, first.err)
	}
	if !errors.Is(second.err, unchanged) || !errors.Is(second.err, durable) {
		t.Fatalf("second write: %v", second.err)
	}
}

// TestQueueDirtyErrorAfterBatchChange checks that a write failing after it
// changed a batch an earlier write changed runs again once that write
// commits, then fails alone without committing its change.
func TestQueueDirtyErrorAfterBatchChange(t *testing.T) {
	// Open the World and queue a create and a failing create behind it.
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithChangelog())
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	q := New(tb.Engine, Options{})
	create := func(key string, fail error, runs *int) func(context.Context, world.Tx) error {
		return func(ctx context.Context, tx world.Tx) error {
			*runs++
			object, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(object)
			if err != nil {
				return err
			}
			return fail
		}
	}
	failed := errors.New("failed after its change")
	var firstRuns, secondRuns int
	first := &queuedWrite{ctx: ctx, arrival: 1, done: make(chan struct{}), name: func() string { return "first" }}
	first.body = create("first", nil, &firstRuns)
	second := &queuedWrite{ctx: ctx, arrival: 2, done: make(chan struct{}), name: func() string { return "second" }}
	second.body = create("second", failed, &secondRuns)
	q.pending, q.draining = []*queuedWrite{first, second}, true
	go q.drain()

	// The first commits once its prefix batch runs; the second then fails
	// alone, and its change never commits.
	<-first.done
	<-second.done
	if first.err != nil || !errors.Is(second.err, failed) {
		t.Fatalf("first %v, second %v", first.err, second.err)
	}
	if firstRuns != 2 || secondRuns != 2 {
		t.Fatalf("first ran %d times and second %d, want 2 each", firstRuns, secondRuns)
	}
	tx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for key, want := range map[string]bool{"first": true, "second": false} {
		if exists, err := tx.HasObject(ctx, key); err != nil || exists != want {
			t.Fatal(key, exists, err)
		}
	}
}
