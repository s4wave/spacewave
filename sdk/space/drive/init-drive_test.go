package s4wave_drive_test

import (
	"context"
	"testing"
	"time"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_drive "github.com/s4wave/spacewave/sdk/space/drive"
	"github.com/sirupsen/logrus"
)

func newDriveWorld(t *testing.T) (context.Context, world.WorldState, func(), peer.ID) {
	t.Helper()

	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		btb.Release()
		t.Fatal(err)
	}

	cleanup := func() {
		wtb.Release()
		btb.Release()
	}

	return ctx, world.NewEngineWorldState(wtb.Engine, true), cleanup, wtb.Volume.GetPeerID()
}

func TestInitDriveCreatesTypedDriveForUnixFSRoot(t *testing.T) {
	ctx, ws, cleanup, sender := newDriveWorld(t)
	defer cleanup()

	ts := time.Now()
	if _, _, err := space_world_ops.InitUnixFS(ctx, ws, sender, "files", ts); err != nil {
		t.Fatal(err)
	}

	roots := []*s4wave_drive.DriveRoot{
		{
			RootId:        "default",
			Name:          "My Files",
			RootObjectKey: "files",
			RootType:      unixfs_world.FSNodeTypeID,
		},
	}
	if _, _, err := s4wave_drive.InitDrive(ctx, ws, sender, "drive", "Drive", roots, ts); err != nil {
		t.Fatal(err)
	}

	typeID, err := world_types.GetObjectType(ctx, ws, "drive")
	if err != nil {
		t.Fatal(err)
	}
	if typeID != s4wave_drive.DriveTypeID {
		t.Fatalf("expected Drive type %q, got %q", s4wave_drive.DriveTypeID, typeID)
	}

	obj, found, err := ws.GetObject(ctx, "drive")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("drive object not found")
	}

	var state *s4wave_drive.Drive
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		state, err = s4wave_drive.UnmarshalDrive(ctx, bcs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.GetDisplayName() != "Drive" {
		t.Fatalf("unexpected display name: %q", state.GetDisplayName())
	}
	if got := len(state.GetRoots()); got != 1 {
		t.Fatalf("expected one drive root, got %d", got)
	}
	if state.GetRoots()[0].GetRootObjectKey() != "files" {
		t.Fatalf("unexpected root object key: %q", state.GetRoots()[0].GetRootObjectKey())
	}
	if state.GetRoots()[0].GetAddedAt() == nil {
		t.Fatal("expected root added_at to be populated")
	}
}

func TestInitDriveRejectsMissingBackingRoot(t *testing.T) {
	ctx, ws, cleanup, sender := newDriveWorld(t)
	defer cleanup()

	roots := []*s4wave_drive.DriveRoot{
		{
			RootId:        "default",
			Name:          "My Files",
			RootObjectKey: "missing",
			RootType:      unixfs_world.FSNodeTypeID,
		},
	}
	if _, _, err := s4wave_drive.InitDrive(ctx, ws, sender, "drive", "Drive", roots, time.Now()); err == nil {
		t.Fatal("expected missing backing root to fail")
	}
}
