package space_unixfs

import (
	"context"
	"io"
	"testing"

	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestFSCursorProjectsGitRepoMetadataPaths(t *testing.T) {
	// Create the context and debug logger for the repository metadata test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the projected World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine and retain it for the test lifetime.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register Git operations on the World engine.
	gitOpc := world.NewLookupOpController("test-space-projection-git-repo", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Initialize the Git repository in the writable World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo/demo", nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Open the projected filesystem root and retain its handle.
	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 10, "space-git-repo")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Open the repository metadata mount through the shared object path.
	repoHandle, _, err := rootHandle.LookupPath(ctx, "u/10/so/space-git-repo/-/repo/demo/-")
	if err != nil {
		t.Fatal(err)
	}
	defer repoHandle.Release()

	// Read the projected repository metadata directory entries.
	var names []string
	if err := repoHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Verify that the projection includes every required metadata entry.
	expected := map[string]struct{}{
		"HEAD":        {},
		"config":      {},
		"description": {},
		"hooks":       {},
		"info":        {},
		"objects":     {},
		"refs":        {},
	}
	for _, name := range names {
		delete(expected, name)
	}
	for name := range expected {
		t.Fatalf("missing repository metadata entry %q in %v", name, names)
	}

	// Open the projected repository HEAD.
	headHandle, _, err := rootHandle.LookupPath(ctx, "u/10/so/space-git-repo/-/repo/demo/-/HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer headHandle.Release()

	// Read the projected HEAD contents.
	buf := make([]byte, 64)
	n, err := headHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Verify that the projected HEAD points to the master branch.
	if got := string(buf[:n]); got != "ref: refs/heads/master\n" {
		t.Fatalf("got HEAD %q, want %q", got, "ref: refs/heads/master\n")
	}
}
