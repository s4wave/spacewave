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
	fsHandle, err := unixfs_world_testbed.InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}

	srcRoot := path.Join(wd, "../../")
	err = SyncFromGitWorkdir(ctx, fsHandle, srcRoot, unixfs_sync.DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	le.Info("synchronized git files to workdir successfully")
	_ = fsHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		le.Debugf("file: %s", ent.GetName())
		return nil
	})
}
