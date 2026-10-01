package space_unixfs

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

func TestFSCursorRejectsInvalidGitRepoProjection(t *testing.T) {
	// Set up a debug logger for the testbeds.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a hydra testbed and a World engine testbed on top of it.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start a World engine testbed on top of the hydra testbed.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Create a World object and label it as a git repo.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	{
		createdObject, err := ws.CreateObject(ctx, "repo/invalid", &bucket.ObjectRef{})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := world_types.SetObjectType(ctx, ws, "repo/invalid", git_world.GitRepoTypeID); err != nil {
		t.Fatal(err)
	}

	// Open a read-write FS cursor and its FS handle over the World.
	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 13, "space-git-invalid")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Reject a lookup into the invalid git repo projection.
	invalidHandle, _, err := rootHandle.LookupPath(ctx, "u/13/so/space-git-invalid/-/repo/invalid/-/HEAD")
	if err == nil {
		invalidHandle.Release()
		t.Fatal("expected invalid git repo projection to fail")
	}
}
