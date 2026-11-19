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
	// Refuse an attempt when the test invalidates its fork.
	if err := t.beforeAdmit(); err != nil {
		return nil, err
	}

	// Let the real World commit the surviving attempt.
	return t.Transaction.Admit(ctx)
}

// TestQueueRetriesCleanErrorAfterDiscard checks that an unchanged write
// returns the outcome of the attempt whose predecessor actually committed.
func TestQueueRetriesCleanErrorAfterDiscard(t *testing.T) {
	// Open a World and hold the retry before it can commit.
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
	discarded := errors.New("discarded")
	durable := errors.New("durable")
	keyForAttempt := func() error {
		if admissions == 0 {
			return discarded
		}
		return durable
	}
	q := New(tb.Engine, Options{
		Unchanged: unchanged,
		Wrap: func(ctx context.Context, tx world.Tx) Transaction {
			return &gatedAdmitTx{Transaction: wrapBasic(ctx, tx), beforeAdmit: func() error {
				// Drop the first fork and wait for the test to release the retry.
				admissions++
				if admissions == 1 {
					return coord.ErrStaleGeneration
				}

				// Admit the retry only after the test inspects its caller.
				close(retrying)
				<-gate
				return nil
			}}
		},
	})

	// Seed the queue with both callers waiting before the drain takes a batch.
	first := &queuedWrite{ctx: ctx, arrival: 1, done: make(chan struct{}), name: func() string { return "create" }}
	first.body = func(ctx context.Context, tx world.Tx) error {
		// Stage a different key in each attempt so the reader's outcome changes.
		created++
		object, err := tx.CreateObject(ctx, keyForAttempt().Error(), nil)
		world.ReleaseObjectState(object)
		return err
	}
	second := &queuedWrite{ctx: ctx, arrival: 2, done: make(chan struct{}), name: func() string { return "check" }}
	second.body = func(ctx context.Context, tx world.Tx) error {
		// Tie this outcome to the first write's tentative change.
		checked++
		key := keyForAttempt()
		found, err := tx.HasObject(ctx, key.Error())
		if err != nil {
			return err
		}
		if !found {
			return errors.New("batch reader missed its predecessor")
		}

		// Distinguish the retry's clean result from the discarded result.
		return errors.Join(unchanged, key)
	}
	q.pending, q.draining = []*queuedWrite{first, second}, true
	go q.drain()

	// The retry has run both bodies, and the caller still has no result.
	<-retrying
	if created != 2 || checked != 2 {
		t.Fatalf("the clean error split the batch: %d creates and %d checks", created, checked)
	}
	if second.ended() {
		t.Fatalf("second write returned before the retry committed: %v", second.err)
	}
	close(release)
	release = nil

	// The retry commits the new value, and the second caller sees that outcome.
	<-first.done
	<-second.done
	if first.err != nil || first.seqno == 0 {
		t.Fatalf("first write: revision %d, error %v", first.seqno, first.err)
	}
	if !errors.Is(second.err, unchanged) || !errors.Is(second.err, durable) {
		t.Fatalf("second write: %v", second.err)
	}
}
