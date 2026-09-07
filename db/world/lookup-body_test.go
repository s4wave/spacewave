package world_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// bodyOnlyWorld exposes the remote body-read contract and rejects object handles.
type bodyOnlyWorld struct {
	world.WorldState
	inner        world.WorldState
	batches      int
	bodyError    error
	allowHandles bool
	handles      int
}

func (w *bodyOnlyWorld) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	w.handles++
	if !w.allowHandles {
		return nil, false, errors.New("body lookup allocated an object handle")
	}
	return w.inner.GetObject(ctx, key)
}

func (w *bodyOnlyWorld) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*world.ObjectBody, error) {
	w.batches++
	if w.bodyError != nil {
		return nil, w.bodyError
	}
	return world.GetObjectBodiesBatch(ctx, w.inner, keys)
}

// TestLookupObjectBodyUsesBodyRead preserves values and missing errors without resource allocation.
func TestLookupObjectBodyUsesBodyRead(t *testing.T) {
	// Seed a real World object behind the body-only transport boundary.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	state, _, err := world.CreateWorldObject(ctx, ws, "example/body", func(cursor *block.Cursor) error {
		cursor.SetBlock(block_mock.NewExample("retained value"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(state)

	// The singular typed read uses the same immutable body contract as batch reads.
	reader := &bodyOnlyWorld{inner: ws}
	value, err := world.LookupObjectBody[*block_mock.Example](ctx, reader, "example/body", block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err)
	}
	if value.GetMsg() != "retained value" || reader.batches != 1 {
		t.Fatalf("body = %v, requests = %d", value, reader.batches)
	}
	if _, err := world.LookupObjectBody[*block_mock.Example](ctx, reader, "missing", block_mock.NewExampleBlock); !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("missing object error = %v", err)
	}

	// A smaller unary envelope cannot make an otherwise readable object unavailable.
	reader.allowHandles = true
	for index, transportError := range []error{
		&world.ObjectBodyTooLargeError{ObjectKey: "example/body", EncodedSize: 200, ByteBudget: 100},
		errors.New("remote response exceeds unary envelope"),
	} {
		reader.bodyError = transportError
		value, err = world.LookupObjectBody[*block_mock.Example](ctx, reader, "example/body", block_mock.NewExampleBlock)
		if err != nil || value.GetMsg() != "retained value" || reader.handles != index+1 {
			t.Fatalf("oversized body = %v, handles = %d, error = %v", value, reader.handles, err)
		}
	}
}

var _ world.ObjectBodyBatcher = (*bodyOnlyWorld)(nil)
