package world_block_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// TestBuildWorldStateFromCursorLocalObjectRef checks that an object root in
// the World's own bucket is stored as a block edge of the World DAG, so block
// walks and garbage collection reach the object's content.
func TestBuildWorldStateFromCursorLocalObjectRef(t *testing.T) {
	// Start the storage testbed.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Build the World on an empty cursor.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ocs.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Discard()

	// Write a root in the World's bucket. An importer holding its external ref
	// creates objects from refs that reference the bucket.
	written, err := world_block.BuildMockObject(ctx, ws, "written")
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(written)

	// Reference the bucket on the written ref and create the imported
	// object.
	bucketID := ocs.GetRefWithOpArgs().GetBucketId()
	external, _, err := written.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	external = external.Clone()
	external.BucketId = bucketID
	obj, err := ws.CreateObject(ctx, "imported", external)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)

	// The stored Object references no bucket and references its root.
	root, err := obj.(*world_block.ObjectState).GetRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored := root.GetRootRef().GetBucketId(); stored != "" {
		t.Fatalf("stored object root names bucket %q, want a local edge", stored)
	}
	refs, err := root.GetRootRef().GetBlockRefs()
	if err != nil {
		t.Fatal(err)
	}
	if refs[1].GetEmpty() {
		t.Fatal("stored object root is not a block ref")
	}

	// The public ref still references the World's bucket.
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetBucketId() != bucketID {
		t.Fatalf("object ref bucket = %q, want %q", ref.GetBucketId(), bucketID)
	}

	// Empty roots use the same bucket normalization with or without a root pointer.
	for _, test := range []struct {
		key string
		ref *bucket.ObjectRef
	}{
		{key: "empty-nil", ref: &bucket.ObjectRef{BucketId: bucketID}},
		{key: "empty-pointer", ref: &bucket.ObjectRef{BucketId: bucketID, RootRef: &block.BlockRef{}}},
	} {
		empty, err := ws.CreateObject(ctx, test.key, test.ref)
		if err != nil {
			t.Fatal(err)
		}
		root, err := empty.(*world_block.ObjectState).GetRoot(ctx)
		ref, _, refErr := empty.GetRootRef(ctx)
		world.ReleaseObjectState(empty)
		if err != nil || refErr != nil {
			t.Fatalf("%s: root=%v ref=%v", test.key, err, refErr)
		}
		if root.GetRootRef().GetBucketId() != "" || ref.GetBucketId() != bucketID {
			t.Fatalf("%s: stored=%v public=%v", test.key, root.GetRootRef(), ref)
		}
		if test.ref.GetBucketId() != bucketID {
			t.Fatalf("%s: normalization mutated its input", test.key)
		}
	}
}
