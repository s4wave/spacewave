package world_block

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestEngineTxWaitSeqnoFollowsPendingWriter proves pending mutations wake their own waiters.
func TestEngineTxWaitSeqnoFollowsPendingWriter(t *testing.T) {
	// Capture a pending writer's sequence while the engine head remains unchanged.
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)
	writer, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writer.Discard)
	before, err := writer.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for exactly one future pending revision without committing the writer.
	result := make(chan error, 1)
	go func() {
		seqno, err := writer.WaitSeqno(ctx, before+1)
		if err == nil && seqno <= before {
			t.Errorf("pending sequence = %d, want greater than %d", seqno, before)
		}
		result <- err
	}()
	obj, err := writer.CreateObject(ctx, "wait/pending", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}

	// The live head must still describe the unmodified accepted World.
	head, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if head != before {
		t.Fatalf("pending mutation advanced head from %d to %d", before, head)
	}
}

// TestEngineTxWaitSeqnoKeepsReadSnapshot proves live head advancement cannot satisfy a snapshot wait.
func TestEngineTxWaitSeqnoKeepsReadSnapshot(t *testing.T) {
	// Open an immutable read revision before another transaction publishes an object.
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)
	reader, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.Discard)
	before, err := reader.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Publish a later object through a separate accepted write transaction.
	writer, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writer.Discard)
	obj, err := writer.CreateObject(ctx, "wait/later", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// A canceled wait beyond the snapshot cannot return the newer accepted head.
	waitCtx, cancel := context.WithCancel(ctx)
	cancel()
	if seqno, err := reader.WaitSeqno(waitCtx, before+1); !errors.Is(err, context.Canceled) {
		t.Fatalf("immutable wait = (%d, %v), want cancellation", seqno, err)
	}
}
