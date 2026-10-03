package unixfs_world_testbed

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestFsInitAdoptsExistingRoot checks that FsInit with an fsRef points the new
// object at that filesystem root instead of creating an empty one.
func TestFsInitAdoptsExistingRoot(t *testing.T) {
	// Build a World on a fresh testbed.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err)
	}
	ws := world.NewEngineWorldState(wtb.Engine, true)

	// Create the source filesystem and read its root.
	sender := htb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	created := time.Unix(1000, 0)
	if _, _, err := unixfs_world.FsInit(ctx, ws, sender, "fs/source", fsType, nil, false, created); err != nil {
		t.Fatal(err)
	}
	sourceRef := readRootRef(ctx, t, ws, "fs/source")

	// Initialize a second object from the source root at a later time, which
	// would produce a different root if FsInit created a new filesystem.
	if _, _, err := unixfs_world.FsInit(ctx, ws, sender, "fs/adopted", fsType, sourceRef, false, created.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	adoptedRef := readRootRef(ctx, t, ws, "fs/adopted")
	if !adoptedRef.EqualVT(sourceRef) {
		t.Fatalf("adopted root %v, want source root %v", adoptedRef, sourceRef)
	}
}

// readRootRef returns the root reference of the World object at objKey.
func readRootRef(ctx context.Context, t *testing.T, ws world.WorldState, objKey string) *bucket.ObjectRef {
	// Report failures at the caller and look up the object.
	t.Helper()
	obj, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("object %q not found", objKey)
	}
	defer world.ReleaseObjectState(obj)

	// Read its root reference.
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
