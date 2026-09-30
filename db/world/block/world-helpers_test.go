package world_block

import (
	"context"
	"testing"

	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// newTestWorld builds a writable mock World on an empty testbed cursor.
// The returned function discards the World and releases the testbed.
func newTestWorld(t *testing.T, ctx context.Context) (*WorldState, *bucket_lookup.Cursor, func()) {
	// Build the testbed and its empty cursor.
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}

	// Build the World on the cursor.
	ws, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tb.Release()
		t.Fatal(err.Error())
	}

	// Release the World, the cursor and the testbed together.
	cleanup := func() {
		ws.Discard()
		ocs.Release()
		tb.Release()
	}
	return ws, ocs, cleanup
}

// writeTestBlock writes an example block and returns its object reference.
func writeTestBlock(t *testing.T, ctx context.Context, ocs *bucket_lookup.Cursor, msg string) *bucket.ObjectRef {
	// Write the block as the root of a new transaction.
	t.Helper()
	btx, bcs := ocs.BuildTransaction(nil)
	bcs.SetBlock(&block_mock.Example{Msg: msg}, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	return &bucket.ObjectRef{RootRef: rootRef}
}

// createTestObject creates an empty object and releases its state.
func createTestObject(t *testing.T, ctx context.Context, ws *WorldState, key string) {
	t.Helper()
	obj, err := ws.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
}
