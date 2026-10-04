package unixfs_sync_git

import (
	"context"
	"os"
	"path"
	"testing"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestSyncFromGitWorkdir(t *testing.T) {
	// Select the test filesystem object to populate from the repository.
	objKey := "test/fs"

	// Prepare logging and the testbed lifecycle context.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the synchronized filesystem.
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Attach a World testbed to the storage testbed.
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Initialize a watched UnixFS object in the World.
	watchWorldChanges := true
	fsHandle, err := unixfs_world_testbed.InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Resolve the repository worktree containing this package.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Synchronize tracked Git files into the test filesystem.
	srcRoot := path.Join(wd, "../../")
	err = SyncFromGitWorkdir(ctx, fsHandle, srcRoot, unixfs_sync.DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Log successful sync and enumerate the resulting root entries.
	le.Info("synchronized git files to workdir successfully")
	_ = fsHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		le.Debugf("file: %s", ent.GetName())
		return nil
	})
}
