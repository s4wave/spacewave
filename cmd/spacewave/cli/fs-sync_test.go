//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// TestSyncFsDir uploads a local directory into an existing UnixFS
// subdirectory in one write transaction and rejects a missing one.
func TestSyncFsDir(t *testing.T) {
	// Build a World holding a UnixFS object.
	ctx := t.Context()
	btb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	root, wtb, err := unixfs_world_testbed.BuildTestbed(btb, "files", false)
	if err != nil {
		t.Fatal(err)
	}
	root.Release()

	// Create d through the engine.
	ref := &unixfs_world.UnixfsRef{ObjectKey: "files"}
	fs, err := unixfs_world.BuildFSFromUnixfsRef(ctx, nil, wtb.WorldState, "", ref, false, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Release()
	if err := unixfs_billy.NewBillyFS(ctx, fs, "", time.Now()).MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}

	// Write the local source directory.
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Upload it into d and commit once.
	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	uri := fsURI{objectKey: "files", path: "d"}
	if err := syncFsDir(ctx, tx, uri, src, true, unixfs_sync.DeleteMode_DeleteMode_AFTER); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Check the committed World holds the file.
	if got := readFsFile(t, ctx, wtb.Engine, "files", "d/a"); got != "hello" {
		t.Fatalf("d/a = %q", got)
	}

	// Check a missing directory fails without changes.
	tx, err = wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	uri.path = "missing"
	err = syncFsDir(ctx, tx, uri, src, true, unixfs_sync.DeleteMode_DeleteMode_AFTER)
	if err == nil || !strings.Contains(err.Error(), "does not exist: missing") {
		t.Fatalf("sync to a missing directory = %v", err)
	}
}

// readFsFile reads a file from a UnixFS object in a read transaction.
func readFsFile(t *testing.T, ctx context.Context, engine world.Engine, objectKey, path string) string {
	// Open the UnixFS object in a read transaction.
	t.Helper()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	ref := &unixfs_world.UnixfsRef{ObjectKey: objectKey}
	fs, err := unixfs_world.BuildFSFromUnixfsRef(ctx, nil, tx, "", ref, false, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Release()

	// Read the file through the billy interface.
	data, err := billy_util.ReadFile(unixfs_billy.NewBillyFS(ctx, fs, "", time.Time{}), path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
