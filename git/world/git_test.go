//go:build test_git_clone_world
// +build test_git_clone_world

package git_world

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	bucket "github.com/aperturerobotics/hydra/bucket"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/aperturerobotics/hydra/testbed"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
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
				le.Debugf(
					"%v %s",
					f.Mode().String(),
					f.Name(),
				)
			}
			le.Info("showing git status")
			status, err := wt.Status()
			if err != nil {
				return err
			}
			statusStr := status.String()
			if statusStr == "" {
				le.Debug("status: clean")
			} else {
				le.Debug(status.String())
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
