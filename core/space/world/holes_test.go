package space_world_test

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
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
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
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

// TestHolesGitCheckout clones a repository and checks out worktrees over a
// Space World, with the scratch hole check on every PendingStore store.
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
			src := holesRepo(t, 40, 48<<10)

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

			// Check out worktrees, each into its own workdir, as at 18:24.
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

			// Remount and read every object, so a hole a later flush
			// would store shows here too.
			eng2, ref2 := f.remount(ctx, t, eng, ref)
			ref = ref2
			waitObjects(ctx, t, eng2, "repo", "repo2", "wt-0", "wt-1", "wt-2")
		})
	}
}

// TestHolesStagedAdoption writes blobs through an engine stage, adopts each
// root with a plain transaction as GLaDOS's worldbatch does, releases the
// stage, and keeps committing so checkpoints and sweeps run, then reads every
// blob after a remount.
func TestHolesStagedAdoption(t *testing.T) {
	for _, tc := range []struct {
		name       string
		frees      bool
		maxPending uint64
	}{
		{"local", false, 0},
		{"frees", true, 0},
		{"frees-overflow", true, 64 << 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAdapterFixtureOn(t, fixtureOpts{frees: tc.frees, maxPending: tc.maxPending})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			eng, ref := f.start(t)
			defer func() { ref.Release() }()
			const n = 16
			for i := range n {
				// Stage a blob, adopt its root, then release the stage.
				stage, err := eng.StageWorldState(ctx)
				if err != nil {
					t.Fatal(err)
				}
				data := make([]byte, 96<<10)
				_, _ = rand.Read(data)
				oref, err := world.AccessObject(ctx, stage.AccessWorldState, nil, func(bcs *block.Cursor) error {
					_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				key := "staged-" + strconv.Itoa(i)
				if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
					obj, err := ws.CreateObject(ctx, key, oref)
					world.ReleaseObjectState(obj)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				stage.Release()

				// Commit unrelated objects so checkpoints install and sweep.
				for j := range 3 {
					if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
						obj, err := ws.CreateObject(ctx, "filler-"+strconv.Itoa(i)+"-"+strconv.Itoa(j), nil)
						world.ReleaseObjectState(obj)
						return err
					}); err != nil {
						t.Fatal(err)
					}
				}
				if i%4 == 3 {
					f.collect(t)
				}
			}

			// Remount and read every blob in full.
			eng2, ref2 := f.remount(ctx, t, eng, ref)
			ref = ref2
			waitObjects(ctx, t, eng2, "staged-"+strconv.Itoa(n-1))
			tx, err := eng2.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			for i := range n {
				key := "staged-" + strconv.Itoa(i)
				obj, found, err := tx.GetObject(ctx, key)
				if err != nil || !found {
					t.Fatalf("%s: found=%v err=%v", key, found, err)
				}
				root, _, err := obj.GetRootRef(ctx)
				world.ReleaseObjectState(obj)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := world.AccessObject(ctx, tx.AccessWorldState, root, func(bcs *block.Cursor) error {
					r, err := blob.NewReader(ctx, bcs)
					if err != nil {
						return err
					}
					got, err := io.ReadAll(r)
					if err == nil && len(got) != 96<<10 {
						err = errors.New("short blob")
					}
					return err
				}); err != nil {
					t.Fatalf("reading %s: %v", key, err)
				}
			}
		})
	}
}

// TestHolesKeypair stores keypairs as forge worker creation does, inside a
// transaction, with commits, collections and a remount between them, then
// reads every keypair body.
func TestHolesKeypair(t *testing.T) {
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	eng, ref := f.start(t)
	defer func() { ref.Release() }()
	var keys []string
	for i := range 12 {
		_, _, pub, err := peer.NewPeerWithGenerateED25519()
		if err != nil {
			t.Fatal(err)
		}
		kp, err := identity.NewKeypair(pub, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, identity_world.NewKeypairKey(kp.GetPeerId()))
		if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			_, _, err := identity_world.StoreKeypair(ctx, ws, "", kp, false)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for j := range 3 {
			if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
				obj, err := ws.CreateObject(ctx, "kfiller-"+strconv.Itoa(i)+"-"+strconv.Itoa(j), nil)
				world.ReleaseObjectState(obj)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
		if i%3 == 2 {
			f.collect(t)
		}
	}
	eng2, ref2 := f.remount(ctx, t, eng, ref)
	ref = ref2
	waitObjects(ctx, t, eng2, keys...)
	if err := world.ExecTransaction(ctx, eng2, false, func(ctx context.Context, ws world.WorldState) error {
		for _, key := range keys {
			kp, err := identity_world.LookupKeypairBody(ctx, ws, key)
			if err != nil {
				return errors.New(key + ": " + err.Error())
			}
			if kp == nil {
				return errors.New(key + ": missing")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
