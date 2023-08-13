package unixfs_sync_git

import (
	"context"
	"os"
	"path"
	"testing"

	hydra_testbed "github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestSyncFromGitWorkdir(t *testing.T) {
	objKey := "test/fs"

	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := true
	fsHandle, err := unixfs_world.InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}

	repoRoot := path.Join(wd, "../../../")
	err = SyncFromGitWorkdir(ctx, fsHandle, repoRoot, unixfs_sync.DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	le.Info("synchronized git files to workdir successfully")
	_ = fsHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		le.Debugf("file: %s", ent.GetName())
		return nil
	})
}
