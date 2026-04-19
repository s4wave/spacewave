//go:build test_git_clone_world

package git_world

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
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
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/util"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

func newGitWorldState(t *testing.T) (context.Context, *logrus.Entry, world.WorldState, func(), peer.ID) {
	t.Helper()

	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"
	bucketID := tb.BucketId

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test/git: git-worktree-batch_test.go", []byte(objectStoreID), encKey)

	xfrmConf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	initWorldRef := &bucket.ObjectRef{
		BucketId:      bucketID,
		TransformConf: xfrmConf,
	}
	_, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID,
			bucketID,
			objectStoreID,
			initWorldRef,
			xfrmConf,
			false,
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	opc := world.NewLookupOpController("test-git-ops", engineID, LookupGitOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()
	<-time.After(time.Millisecond * 100)

	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	ws := world.NewEngineWorldState(busEngine, true)
	cleanup := func() {
		worldCtrlRef.Release()
		tb.Release()
	}
	return ctx, le, ws, cleanup, tb.Volume.GetPeerID()
}

func createSourceRepo(t *testing.T) (string, plumbing.Hash, plumbing.Hash) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{
		Name:  "Test",
		Email: "test@example.com",
		When:  time.Now(),
	}
	if err := os.WriteFile(filepath.Join(dir, "one.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("one.txt"); err != nil {
		t.Fatal(err)
	}
	firstHash, err := wt.Commit("first", &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Remove("one.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dir", "two.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("dir/two.txt"); err != nil {
		t.Fatal(err)
	}
	secondHash, err := wt.Commit("second", &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir, firstHash, secondHash
}

func assertWorktreeState(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	repoObjKey, worktreeObjKey string,
	ts time.Time,
	sender peer.ID,
	wantFiles map[string]string,
	wantMissing []string,
) {
	t.Helper()

	err := AccessWorldObjectRepoWithWorktree(
		ctx,
		le,
		ws,
		repoObjKey,
		worktreeObjKey,
		ts,
		false,
		sender,
		func(repo *git.Repository, workDir billy.Filesystem) error {
			wt, err := repo.Worktree()
			if err != nil {
				return err
			}
			status, err := wt.Status()
			if err != nil {
				return err
			}
			for path, fs := range status {
				if fs.Staging != git.Unmodified || fs.Worktree != git.Unmodified {
					return errors.Errorf("worktree not clean for %s: %c%c", path, fs.Staging, fs.Worktree)
				}
			}
			for path, want := range wantFiles {
				data, err := util.ReadFile(workDir, path)
				if err != nil {
					return err
				}
				if got := string(data); got != want {
					return errors.Errorf("file %s = %q, want %q", path, got, want)
				}
			}
			for _, path := range wantMissing {
				if _, err := workDir.Stat(path); err == nil {
					return errors.Errorf("expected %s to be absent", path)
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGitWorktreeBatchCheckout(t *testing.T) {
	ctx, le, ws, cleanup, sender := newGitWorldState(t)
	defer cleanup()

	repoDir, firstHash, _ := createSourceRepo(t)
	repoKey := "repo/test"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	opTs := unixfs_block.FillPlaceholderTimestamp(nil)
	ts := opTs.AsTime()

	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: workdirKey,
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	_, err := GitClone(
		ctx,
		ws,
		repoKey,
		sender,
		&git_block.CloneOpts{
			Url: repoDir,
		},
		nil,
		nil,
		&GitCreateWorktreeOp{
			ObjectKey:     worktreeKey,
			CreateWorkdir: true,
			WorkdirRef:    workdirRef,
			CheckoutOpts: &git_block.CheckoutOpts{
				Force: true,
			},
			Timestamp: opTs,
		},
		opTs,
	)
	if err != nil {
		t.Fatal(err)
	}

	assertWorktreeState(
		t,
		ctx,
		le,
		ws,
		repoKey,
		worktreeKey,
		ts,
		sender,
		map[string]string{"dir/two.txt": "two\n"},
		[]string{"one.txt"},
	)

	firstCommit, err := git_block.NewHash(firstHash)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ws.ApplyWorldOp(
		ctx,
		&GitWorktreeCheckoutOp{
			ObjectKey:     worktreeKey,
			RepoObjectKey: repoKey,
			CheckoutOpts: &git_block.CheckoutOpts{
				Commit: firstCommit,
				Force:  true,
			},
			Timestamp: opTs,
		},
		sender,
	)
	if err != nil {
		t.Fatal(err)
	}

	assertWorktreeState(
		t,
		ctx,
		le,
		ws,
		repoKey,
		worktreeKey,
		ts,
		sender,
		map[string]string{"one.txt": "one\n"},
		[]string{"dir/two.txt"},
	)
}
