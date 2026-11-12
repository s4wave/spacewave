package space_world_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
)

// errAdmitCrash stands for a crash at an Admit step.
var errAdmitCrash = errors.New("crash at admit step")

// TestAdmitOverflowHeld tries to lose a block Admit put past the
// PendingStore's bound. A commit far larger than the bound, with no payloads,
// runs with a collection at a 1 ns grace after its put, after its hold, after
// its stage is released and after it returns, a commit of another repository
// crashes before its hold, and after a collection and a remount every block of
// the first must still read.
func TestAdmitOverflowHeld(t *testing.T) {
	lookup := world.NewSchema(git_world.GitTypes, unixfs_world.UnixfsTypes).LookupOp
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, maxPending: 64 << 10, opTypes: &space_world.OpTypes{Lookup: lookup}})
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	eng, ref := f.start(t)
	defer func() { ref.Release() }()
	ws := world.NewEngineWorldState(eng, true)
	src := holesRepo(t, 40, 48<<10)
	collector := f.volume.(volume.Collector)
	collect := func() error {
		for range 64 {
			res, err := collector.Collect(ctx)
			if err != nil || !res.More {
				return err
			}
		}
		return errors.New("collection did not finish")
	}

	// Commit the clone, collecting between each step of its Admit. After the
	// stage's release, another writer's plain put first carries the release
	// into the Volume, which defers it to the next changing direct
	// transaction.
	var steps []string
	restore := space_world.SetAdmitStep(func(step string) error {
		steps = append(steps, step)
		if step == "released" {
			data := fmt.Appendf(nil, "another writer %d", len(steps))
			if _, _, err := f.volume.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true}); err != nil {
				return err
			}
		}
		return collect()
	})
	clone := func(key, src string) error {
		_, _, err := ws.ApplyWorldOp(ctx, &git_world.GitCloneOp{
			ObjectKey:       key,
			CloneOpts:       &git_block.CloneOpts{Url: src, DisableCheckout: true},
			DisableCheckout: true,
		}, "")
		return err
	}
	if err := clone("repo", src); err != nil {
		t.Fatal(err)
	}
	restore()
	if !slices.Contains(steps, "released") {
		t.Fatalf("the commit ran Admit steps %v, none after a release", steps)
	}
	if err := collect(); err != nil {
		t.Fatal(err)
	}

	// Read every block of the clone by checking it out, before any replay
	// could store them again.
	checkout := func(ws world.WorldState, wt string) error {
		_, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitCreateWorktreeOp(
			wt, "repo",
			&unixfs_world.UnixfsRef{ObjectKey: wt + "-dir", FsType: unixfs_world.FSType_FSType_FS_NODE},
			true,
			&git_block.CheckoutOpts{Branch: "refs/heads/master", Force: true},
			false,
			time.Now(),
		), "")
		return err
	}
	if err := checkout(ws, "wt"); err != nil {
		t.Fatal(err)
	}

	// Crash a clone of another repository after its put, before its hold.
	restore = space_world.SetAdmitStep(func(step string) error {
		if err := collect(); err != nil {
			return err
		}
		return errAdmitCrash
	})
	if err := clone("repo-crashed", holesRepo(t, 40, 48<<10)); !errors.Is(err, errAdmitCrash) {
		t.Fatalf("crashed clone: %v", err)
	}
	restore()
	if err := collect(); err != nil {
		t.Fatal(err)
	}

	// Remount, collect again, and read every block of the first clone once
	// more.
	eng2, ref2 := f.remount(ctx, t, eng, ref)
	ref = ref2
	waitObjects(ctx, t, eng2, "repo")
	if err := collect(); err != nil {
		t.Fatal(err)
	}
	if err := checkout(world.NewEngineWorldState(eng2, true), "wt2"); err != nil {
		t.Fatal(err)
	}
	if err := world.ExecTransaction(ctx, eng2, false, func(ctx context.Context, ws world.WorldState) error {
		_, found, err := ws.GetObject(ctx, "repo-crashed")
		if err == nil && found {
			err = errors.New("the crashed clone's object exists")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
