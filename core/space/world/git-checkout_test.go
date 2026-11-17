package space_world_test

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	_ "github.com/go-git/go-git/v6/plumbing/transport/file"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world "github.com/s4wave/spacewave/core/space/world"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
)

// holesRepo writes a Git repository with files of random data, so the
// clone and checkout write more than the PendingStore keeps.
func holesRepo(t *testing.T, files, size int) string {
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
	for i := range files {
		name := filepath.Join("d"+strconv.Itoa(i%7), "f"+strconv.Itoa(i))
		buf := make([]byte, size)
		_, _ = rand.Read(buf)
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), buf, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	sig := &object.Signature{Name: "t", Email: "t@t", When: time.Now()}
	if _, err := wt.Commit("c", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestHolesGitCheckout clones a repository twice, by GitCloneOp and through an
// engine stage as the wizard does, collects, checks out three worktrees with a
// collection after each, then remounts and reads every object. Its writes
// outgrow a 64 KiB PendingStore, so with frees on it fails if a block a
// commit stored past the bound loses its hold before a root names it.
func TestHolesGitCheckout(t *testing.T) {
	for _, tc := range []struct {
		name       string
		frees      bool
		maxPending uint64
	}{
		{"local", false, 0},
		{"frees", true, 0},
		{"overflow", false, 64 << 10},
		{"frees-overflow", true, 64 << 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := world.NewSchema(git_world.GitTypes, unixfs_world.UnixfsTypes).LookupOp
			f := newAdapterFixtureOn(t, fixtureOpts{frees: tc.frees, maxPending: tc.maxPending, opTypes: &space_world.OpTypes{Lookup: lookup}})
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
			defer cancel()
			eng, ref := f.start(t)
			defer func() { ref.Release() }()
			ws := world.NewEngineWorldState(eng, true)
			// 12 files of 16 KiB put the clone and the second and third
			// worktrees past the 64 KiB bound on every run, as checks of
			// OverflowStage.Stored show, and keep the run short where a pack
			// inflates byte by byte, as under GoScript.
			src := holesRepo(t, 12, 16<<10)

			// Clone without checkout, as the import does.
			if _, _, err := ws.ApplyWorldOp(ctx, &git_world.GitCloneOp{
				ObjectKey:       "repo",
				CloneOpts:       &git_block.CloneOpts{Url: src, DisableCheckout: true},
				DisableCheckout: true,
			}, ""); err != nil {
				t.Fatal(err)
			}

			// Clone as the wizard does: an engine stage held through the
			// commit that publishes the repository.
			stage, err := eng.StageWorldState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			repoRef, err := s4wave_git.CloneGitRepoToRef(ctx, stage, &git_block.CloneOpts{Url: src}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			wtx, err := eng.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := wtx.ApplyWorldOp(ctx, git_world.NewGitInitOp("repo2", repoRef, true, nil, timestamppb.Now()), ""); err != nil {
				t.Fatal(err)
			}
			if err := wtx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			wtx.Discard()
			stage.Release()
			f.collect(t)

			// Check out worktrees, each into its own workdir.
			for i := range 3 {
				key := "wt-" + strconv.Itoa(i)
				repoKey := "repo"
				if i == 2 {
					repoKey = "repo2"
				}
				op := git_world.NewGitCreateWorktreeOp(
					key, repoKey,
					&unixfs_world.UnixfsRef{ObjectKey: "wd-" + strconv.Itoa(i), FsType: unixfs_world.FSType_FSType_FS_NODE},
					true,
					&git_block.CheckoutOpts{Branch: "refs/heads/master", Force: true},
					false,
					time.Now(),
				)
				if _, _, err := ws.ApplyWorldOp(ctx, op, ""); err != nil {
					t.Fatalf("worktree %d: %v", i, err)
				}
				f.collect(t)
			}

			// Remount and wait for every object.
			eng2, ref2 := f.remount(ctx, t, eng, ref)
			ref = ref2
			waitObjects(ctx, t, eng2, "repo", "repo2", "wt-0", "wt-1", "wt-2")
		})
	}
}
