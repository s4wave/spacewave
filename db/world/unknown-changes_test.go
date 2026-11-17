package world_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestEngineUnknownChangeCount checks that the producer counts one oversized
// span for several subscribers, not every delivery or every known change.
func TestEngineUnknownChangeCount(t *testing.T) {
	// Resolve the running engine under the same access scope production uses.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	err := world.AccessEngine(ctx, tb.Engine, func(ctx context.Context, engine world.Engine) error {
		// Read the producer's counter on the served concrete engine, not its proxy.
		producer, ok := engine.(*world_block.Engine)
		if !ok {
			return errors.New("testbed did not serve a block World engine")
		}
		if count := producer.GetUnknownChangeCount(); count != 0 {
			t.Fatalf("initial unknown count = %d, want 0", count)
		}
		seqno, err := engine.GetSeqno(ctx)
		if err != nil {
			return err
		}

		// Keep two subscribers alive after a known priming change.
		events := []chan bool{make(chan bool, 2), make(chan bool, 2)}
		done := make(chan error, len(events))
		stop := errors.New("unknown span received")
		for _, event := range events {
			go func() {
				done <- world.WatchChanges(ctx, engine, seqno, func(_ uint64, changes *world.ChangeSet) error {
					event <- changes.Unknown
					if changes.Unknown {
						return stop
					}
					return nil
				})
			}()
		}
		createViewObject(t, ctx, engine, "counter/prime")
		for _, event := range events {
			select {
			case unknown := <-event:
				if unknown {
					t.Fatal("known priming change was unknown")
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if count := producer.GetUnknownChangeCount(); count != 0 {
			t.Fatalf("known change count = %d, want 0", count)
		}

		// Exceed the producer's change bound in one atomic transaction.
		createUnknownViewChange(t, ctx, engine)

		// Both deliveries must refer to one shared unknown comparison.
		for _, event := range events {
			select {
			case unknown := <-event:
				if !unknown {
					t.Fatal("oversized span was not unknown")
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		for range events {
			select {
			case err := <-done:
				if !errors.Is(err, stop) {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if count := producer.GetUnknownChangeCount(); count != 1 {
			t.Fatalf("shared unknown count = %d, want 1", count)
		}

		// A new subscription at the current head must retain the lifetime count.
		head, err := engine.GetSeqno(ctx)
		if err != nil {
			return err
		}
		done = make(chan error, 1)
		go func() {
			done <- world.WatchChanges(ctx, engine, head, func(_ uint64, changes *world.ChangeSet) error {
				if changes.Unknown {
					return errors.New("small later change was unknown")
				}
				return stop
			})
		}()
		createViewObject(t, ctx, engine, "counter/later")
		select {
		case err := <-done:
			if !errors.Is(err, stop) {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		if count := producer.GetUnknownChangeCount(); count != 1 {
			t.Fatalf("later unknown count = %d, want 1", count)
		}
		return nil
	})

	// Report any failure from the production access or change-watch interface.
	if err != nil {
		t.Fatal(err)
	}
}

// createUnknownViewChange creates enough records to exceed one precise change set.
func createUnknownViewChange(t *testing.T, ctx context.Context, engine world.Engine) {
	// Keep the entire oversized change in one short-lived write transaction.
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// The public change watch currently bounds a precise set at 4096 entries.
	for i := range 4097 {
		obj, err := tx.CreateObject(ctx, "counter/bulk/"+strconv.Itoa(i), nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
