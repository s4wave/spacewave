//go:build !js && !wasip1

package world_block

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestEngineSyncThroughDeferredDurability checks the per-seqno fence of the
// single-writer deferred mode: it fences only a head the durable head does not
// cover, and a covered seqno returns without a fence.
func TestEngineSyncThroughDeferredDurability(t *testing.T) {
	// Open a deferred-durability engine.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Build an empty world cursor to back the engine.
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Release)

	// Open a deferred engine without an authority.
	eng, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false, WithDeferredDurability())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Error(err)
		}
	})
	commit := func(key string) uint64 {
		t.Helper()

		// Create one object in a write transaction.
		w, err := eng.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Discard()
		obj, err := w.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "test"})
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}

		// Commit it and read the seqno it published.
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		seqno, err := eng.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return seqno
	}

	// A deferred commit leaves the durable seqno behind; SyncThrough
	// catches it up.
	first := commit("first")
	durable, err := eng.SyncThrough(ctx, first)
	if err != nil || durable != first {
		t.Fatalf("SyncThrough(%d) = %d, %v; want %d", first, durable, err, first)
	}

	// A covered seqno returns without a fence, even with a later unsynced commit.
	second := commit("second")
	durable, err = eng.SyncThrough(ctx, first)
	if err != nil || durable != first {
		t.Fatalf("covered SyncThrough = %d, %v; want %d", durable, err, first)
	}
	durable, err = eng.SyncThrough(ctx, 0)
	if err != nil || durable != first {
		t.Fatalf("SyncThrough(0) = %d, %v; want %d", durable, err, first)
	}

	// An uncovered seqno fences the later commit.
	durable, err = eng.SyncThrough(ctx, second)
	if err != nil || durable != second {
		t.Fatalf("SyncThrough(%d) = %d, %v; want %d", second, durable, err, second)
	}

	// A seqno no commit has reached waits for that commit.
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := eng.SyncThrough(waitCtx, second+1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SyncThrough past the head = %v; want deadline exceeded", err)
	}
}
