package world_block

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/world"
)

// TestReadTransactionKeepsRevision checks that publication and storage refresh
// cannot change a caller-held snapshot, with and without a write coordinator.
func TestReadTransactionKeepsRevision(t *testing.T) {
	for _, coordinated := range []bool{false, true} {
		name := "local"
		var options []EngineOption
		if coordinated {
			name = "coordinated"
			options = append(options, WithWriteCoordinator(
				coord_inmem.NewCoordinator(),
				coord.Scope{VolumeID: "snapshot-volume", ObjectStoreID: "snapshot-store"},
				nil,
				func(context.Context) (*bucket.ObjectRef, error) { return nil, nil },
			))
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			engine := newRetirementTestEngine(t, ctx, options...)
			reader, err := engine.NewBlockEngineTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Discard()
			writer, err := engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Discard()
			object, err := writer.CreateObject(ctx, "snapshot/later", nil)
			world.ReleaseObjectState(object)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}

			// Reopen backing storage while retaining the original immutable root.
			if err := reader.refreshReadSnapshot(ctx, reader.readTx.Load()); err != nil {
				t.Fatal(err)
			}
			object, found, err := reader.GetObject(ctx, "snapshot/later")
			world.ReleaseObjectState(object)
			if err != nil || found {
				t.Fatalf("original snapshot contains later object: found=%t err=%v", found, err)
			}
			fork, err := reader.Fork(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer fork.(*Tx).Discard()
			object, found, err = fork.GetObject(ctx, "snapshot/later")
			world.ReleaseObjectState(object)
			if err != nil || found {
				t.Fatalf("fork advanced beyond its source snapshot: found=%t err=%v", found, err)
			}

			fresh, err := engine.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Discard()
			object, found, err = fresh.GetObject(ctx, "snapshot/later")
			world.ReleaseObjectState(object)
			if err != nil || !found {
				t.Fatalf("fresh snapshot lost committed object: found=%t err=%v", found, err)
			}
		})
	}
}

// TestReadSnapshotWaitRev observes live acceptance without moving snapshot reads.
func TestReadSnapshotWaitRev(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	engine := newRetirementTestEngine(t, ctx)
	writer, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	object, err := writer.CreateObject(ctx, "watched", nil)
	world.ReleaseObjectState(object)
	if err != nil {
		writer.Discard()
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	reader, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Discard()
	object, found, err := reader.GetObject(ctx, "watched")
	if err != nil || !found {
		t.Fatalf("read watched object: found=%t err=%v", found, err)
	}
	defer world.ReleaseObjectState(object)
	_, before, err := object.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The live World owns revision watches independently of snapshot readers.
	live := world.NewEngineWorldState(engine, false)
	watch, err := world.MustGetObject(ctx, live, "watched")
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(watch)
	waited := make(chan error, 1)
	go func() {
		_, err := watch.WaitRev(ctx, before+1, false)
		waited <- err
	}()
	writer, err = engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()
	updated, _, err := writer.GetObject(ctx, "watched")
	if err != nil {
		t.Fatal(err)
	}
	_, err = updated.IncrementRev(ctx)
	world.ReleaseObjectState(updated)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-waited; err != nil {
		t.Fatal(err)
	}
	_, after, err := object.GetRootRef(ctx)
	if err != nil || after != before {
		t.Fatalf("wait advanced snapshot: before=%d after=%d err=%v", before, after, err)
	}
	waitCtx, stop := context.WithCancel(ctx)
	stop()
	if _, err := object.WaitRev(waitCtx, before+1, false); err == nil {
		t.Fatal("immutable snapshot observed a later revision")
	}
}
