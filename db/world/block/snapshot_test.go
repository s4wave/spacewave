package world_block_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestBuildSnapshotPreservesFinalWorld verifies packed readback, revisions,
// relationships, and removal of an unreachable body before durable copying.
func TestBuildSnapshotPreservesFinalWorld(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	var removed *bucket.ObjectRef
	var wantSeq uint64
	ref, err := world_block.BuildSnapshot(ctx, tb.Logger, tb.Engine, func(ctx context.Context, state *world_block.WorldState) error {
		for i := range 65 {
			key := fmt.Sprintf("object-%03d", i)
			body, err := world.AccessObject(ctx, state.AccessWorldState, nil, func(cursor *block.Cursor) error {
				cursor.SetBlock(&block_mock.Example{Msg: key}, true)
				return nil
			})
			if err != nil {
				return err
			}
			object, err := state.CreateObject(ctx, key, body)
			world.ReleaseObjectState(object)
			if err != nil {
				return err
			}
			removed = body
		}
		if _, err := state.DeleteObject(ctx, "object-064"); err != nil {
			return err
		}
		if err := state.InsertGraphQuads(ctx, []world.GraphQuad{
			world.NewGraphQuadWithKeys("object-000", "<edge>", "object-001", ""),
			world.NewGraphQuadWithKeys("object-000", "<edge>", "object-000", ""),
		}); err != nil {
			return err
		}
		wantSeq, err = state.GetSeqno(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = tb.Engine.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		state, err := world_block.BuildWorldStateFromCursor(ctx, tb.Logger, false, cursor, tb.Engine, nil, false)
		if err != nil {
			return fmt.Errorf("open World: %w", err)
		}
		defer state.Discard()
		seq, err := state.GetSeqno(ctx)
		if err != nil || seq != wantSeq {
			t.Fatalf("sequence=%d want=%d err=%v", seq, wantSeq, err)
		}
		refs, err := state.GetObjectRootRefsBatch(ctx, []string{"object-000", "object-001", "object-063", "object-064"})
		if err != nil {
			return fmt.Errorf("read object refs: %w", err)
		}
		if refs[0].Rev != 4 || refs[1].Rev != 2 || refs[2].Rev != 1 || refs[3].Exists {
			t.Fatalf("unexpected final object revisions: %v", refs)
		}
		body, err := world.LookupObjectBody[*block_mock.Example](ctx, state, "object-063", block_mock.NewExampleBlock)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if body.Msg != "object-063" {
			t.Fatalf("body=%q", body.Msg)
		}
		quads, err := state.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
		if err != nil || len(quads) != 2 {
			t.Fatalf("relationships=%d err=%v", len(quads), err)
		}
		found, err := cursor.GetBucket().GetBlockExists(ctx, removed.RootRef)
		if found {
			t.Fatal("deleted object body was copied")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBuildSnapshotFailure returns no publishable reference and copies no body
// when population fails after constructing an object through ordinary APIs.
func TestBuildSnapshotFailure(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	want := errors.New("population failed")
	var body *bucket.ObjectRef
	ref, err := world_block.BuildSnapshot(ctx, tb.Logger, tb.Engine, func(ctx context.Context, state *world_block.WorldState) error {
		body, err = world.AccessObject(ctx, state.AccessWorldState, nil, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample("unpublished"), true)
			return nil
		})
		if err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) || ref != nil {
		t.Fatalf("failed snapshot returned ref=%v err=%v", ref, err)
	}
	if err := tb.Engine.AccessWorldState(ctx, body, func(cursor *bucket_lookup.Cursor) error {
		found, err := cursor.GetBucket().GetBlockExists(ctx, body.RootRef)
		if found {
			t.Error("failed snapshot copied its body")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
