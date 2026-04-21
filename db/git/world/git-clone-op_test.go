//go:build test_git_clone_world

package git_world

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	bucket "github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// TestGitCloneOp tests cloning a repo via GitCloneOp through ApplyWorldOp.
func TestGitCloneOp(t *testing.T) {
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
	engineID := "test-clone-op-engine"
	objectStoreID := "test-clone-op-store"
	bucketID := tb.BucketId

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test/git: git-clone-op_test.go", []byte(objectStoreID), encKey)

	xfrmConf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
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

	// register git ops on the bus
	opc := world.NewLookupOpController("test-git-ops", engineID, LookupGitOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()
	select {
	case <-ctx.Done():
		t.Fatal("context canceled waiting for controller")
	case <-time.After(time.Millisecond * 100):
	}

	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	ws := world.NewEngineWorldState(busEngine, true)
	sender := tb.Volume.GetPeerID()

	// Phase 1: Clone via GitCloneOp
	objKey := "gitrepo/hydra"
	cloneOp := &GitCloneOp{
		ObjectKey: objKey,
		CloneOpts: &git_block.CloneOpts{
			Url:             "../../",
			DisableCheckout: true,
		},
		DisableCheckout: true,
	}

	t.Log("cloning via GitCloneOp...")
	seqno, _, err := ws.ApplyWorldOp(ctx, cloneOp, sender)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("clone complete, seqno=%d", seqno)

	// Verify the object exists
	_, exists, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("expected object to exist after clone")
	}
	t.Logf("object %s exists", objKey)

	// Verify we can read the repo (HEAD should be resolvable)
	_, _, err = AccessWorldObjectRepo(ctx, ws, objKey, false, nil, nil, nil, func(repo *git.Repository) error {
		head, err := repo.Head()
		if err != nil {
			return err
		}
		t.Logf("HEAD: %s -> %s", head.Name(), head.Hash())

		iter, err := repo.Log(&git.LogOptions{})
		if err != nil {
			return err
		}
		count := 0
		_ = iter.ForEach(func(c *object.Commit) error {
			count++
			return nil
		})
		t.Logf("commit count: %d", count)
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Phase 2: Fetch via GitFetchOp
	fetchOp := NewGitFetchOp(objKey, &git_block.FetchOpts{
		RemoteUrl: "../../",
	})

	t.Log("fetching via GitFetchOp...")
	seqno2, _, err := ws.ApplyWorldOp(ctx, fetchOp, sender)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("fetch complete, seqno=%d", seqno2)
}
