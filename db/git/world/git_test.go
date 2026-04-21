//go:build test_git_clone_world

package git_world

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	bucket "github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// TestGitClone tests cloning to a world.
func TestGitClone(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"
	bucketID := tb.BucketId

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test/git: git_test.go", []byte(objectStoreID), encKey)

	xfrmConf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// initWorldRef is only used if the world has not been previously inited.
	initWorldRef := &bucket.ObjectRef{
		BucketId:      bucketID,
		TransformConf: xfrmConf,
	}

	// initialize world engine
	_, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			initWorldRef,
			xfrmConf,
			false,
		),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer worldCtrlRef.Release()

	// provide op handlers to bus
	opc := world.NewLookupOpController("test-git-ops", engineID, LookupGitOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()

	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// uses directive to look up the engine
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	// uses short-lived engine txs to implement world state
	ws := world.NewEngineWorldState(busEngine, true)

	sender := tb.Volume.GetPeerID()
	objKey := "test-git-repo"
	worktreeKey := objKey + "/worktree"
	workdirKey := "test-git-workdir"
	opTs := unixfs_block.FillPlaceholderTimestamp(nil)
	ts := opTs.AsTime()
	outRef, err := GitClone(
		ctx,
		ws,
		objKey,
		sender,
		&git_block.CloneOpts{
			Url: "../../",
		},
		nil,
		os.Stderr,
		&GitCreateWorktreeOp{
			ObjectKey:     worktreeKey,
			CreateWorkdir: true,
			WorkdirRef: &unixfs_world.UnixfsRef{
				ObjectKey: workdirKey,
				FsType:    unixfs_world.FSType_FSType_FS_NODE,
			},
			CheckoutOpts: &git_block.CheckoutOpts{
				Force: true,
			},
			Timestamp: opTs,
		},
		opTs,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("cloned to reference: %s", outRef.MarshalString())

	// create alternate worktree at HEAD
	altWorktreeKey := "other-worktree"
	altWorkdirKey := "other-workdir"
	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: altWorkdirKey,
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	le.Info("checking out second worktree")
	err = GitCreateWorktree(
		ctx,
		ws,
		sender,
		altWorktreeKey,
		objKey,
		workdirRef,
		true,
		&git_block.CheckoutOpts{Force: true},
		false,
		ts,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// TODO: remove this delay
	<-time.After(time.Millisecond * 100)

	err = AccessWorldObjectRepoWithWorktree(
		ctx,
		le,
		ws,
		objKey, altWorktreeKey,
		ts, false, sender,
		func(repo *git.Repository, workDir billy.Filesystem) error {
			wt, err := repo.Worktree()
			if err != nil {
				return err
			}
			_ = wt

			le.Info("showing workdir contents")
			files, err := workDir.ReadDir("")
			if err != nil {
				return err
			}
			le.Debugf("workdir contains %d files", len(files))
			for _, f := range files {
				fi, fiErr := f.Info()
				if fiErr != nil {
					le.Debugf("? %s (info err: %v)", f.Name(), fiErr)
					continue
				}
				le.Debugf(
					"%v %s",
					fi.Mode().String(),
					f.Name(),
				)
			}
			le.Info("showing git status")
			status, err := wt.Status()
			if err != nil {
				return err
			}

			// Log and check for symlink-related modifications.
			var modifiedCount int
			for path, fs := range status {
				if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
					le.Debugf("status: %c%c %s", fs.Staging, fs.Worktree, path)
					modifiedCount++
				}
			}
			if modifiedCount == 0 {
				le.Debug("status: clean")
			} else {
				le.Debugf("status: %d files modified", modifiedCount)
			}

			// Check symlinks via Lstat.
			for path, fs := range status {
				if fs.Worktree == git.Modified {
					lfi, lErr := workDir.Lstat(path)
					if lErr != nil {
						le.Debugf("lstat %s: %v", path, lErr)
						continue
					}
					le.Debugf("lstat %s: mode=%v symlink=%v",
						path, lfi.Mode(), lfi.Mode()&os.ModeSymlink != 0)
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test checking out a different reference
	le.Info("checking out different reference")
}
