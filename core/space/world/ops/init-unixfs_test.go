package space_world_ops

import (
	"context"
	"testing"
	"time"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestInitUnixFSCreatesEmptyRoot(t *testing.T) {
	// Prepare the context and logger for the UnixFS initialization fixture.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the empty UnixFS root.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer btb.Release()

	// Start the World engine over the storage testbed.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register the space operation controller with the World engine.
	opc := world.NewLookupOpController("test-space-ops", wtb.EngineID, LookupWorldOp)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}
	<-time.After(100 * time.Millisecond)

	// Initialize the empty UnixFS root in the World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	if _, _, err := InitUnixFS(ctx, ws, wtb.Volume.GetPeerID(), "drive/fs", time.Now()); err != nil {
		t.Fatal(err)
	}

	// Open a UnixFS cursor at the initialized root.
	cursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{ObjectKey: "drive/fs"},
		wtb.Volume.GetPeerID(),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()

	// Open a filesystem handle over the UnixFS cursor.
	handle, err := unixfs_sdk.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Verify the initialized UnixFS root has no directory entries.
	bfs := unixfs_billy.NewBillyFS(ctx, handle, "", time.Now())
	entries, err := bfs.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty UnixFS root, got %d entries", len(entries))
	}
}
