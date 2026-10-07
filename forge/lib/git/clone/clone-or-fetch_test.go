package forge_lib_git_clone

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/pkg/errors"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/forge/testbed"
	"github.com/sirupsen/logrus"
)

// commitFile writes name with body in the source worktree and commits it.
func commitFile(t *testing.T, dir string, wt *git.Worktree, name, body string) plumbing.Hash {
	// Attribute fixture failures to the calling test.
	t.Helper()

	// Stage the file for the next commit.
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatal(err.Error())
	}

	// Commit the staged file.
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}
	hash, err := wt.Commit(name, &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatal(err.Error())
	}
	return hash
}

// TestCloneOrFetchDepthOne clones a repository at depth one, then fetches it
// again after two upstream commits, and checks which commits each run stored.
func TestCloneOrFetchDepthOne(t *testing.T) {
	// Build a source repository with two commits.
	dir := t.TempDir()
	src, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcWt, err := src.Worktree()
	if err != nil {
		t.Fatal(err.Error())
	}
	first := commitFile(t, dir, srcWt, "a.txt", "a\n")
	pinned := commitFile(t, dir, srcWt, "b.txt", "b\n")

	// Start the World testbed.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	ws, sender := tb.WorldState, tb.Volume.GetPeerID()
	le := logrus.NewEntry(logrus.New())

	// Configure a single-branch, tagless, depth-one clone with a worktree.
	conf := &Config{
		ObjectKey: "repo",
		CloneOpts: &git_block.CloneOpts{
			Url:          (&url.URL{Scheme: "file", Path: dir}).String(),
			Depth:        1,
			SingleBranch: true,
			TagMode:      git_block.TagMode_TagMode_NONE,
		},
		WorktreeOpts: &git_world.GitCreateWorktreeOp{
			ObjectKey:     "repo/worktree",
			WorkdirRef:    &unixfs_world.UnixfsRef{ObjectKey: "repo/workdir"},
			CreateWorkdir: true,
		},
	}

	// storedCommits returns the commits held by the repository object.
	storedCommits := func() map[plumbing.Hash]bool {
		commits := make(map[plumbing.Hash]bool)
		_, _, err := git_world.AccessWorldObjectRepo(ctx, ws, "repo", false, nil, nil, nil, func(repo *git.Repository) error {
			iter, err := repo.CommitObjects()
			if err != nil {
				return err
			}
			return iter.ForEach(func(c *object.Commit) error {
				commits[c.Hash] = true
				return nil
			})
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		return commits
	}

	// A single-branch clone of the remote HEAD tracks it as origin/HEAD.
	remoteRef := plumbing.NewRemoteHEADReferenceName("origin")

	// refs returns the worktree's checked-out commit and the remote-tracking tip.
	refs := func() (checkedOut, fetched plumbing.Hash) {
		// Read HEAD through the worktree's own reference store.
		err := git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, "repo", "repo/worktree", time.Now(), false, "", func(repo *git.Repository, _ billy.Filesystem) error {
			head, err := repo.Head()
			if err != nil {
				return err
			}
			checkedOut = head.Hash()
			return nil
		})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Read the remote-tracking ref from the repository.
		_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, "repo", false, nil, nil, nil, func(repo *git.Repository) error {
			remote, err := repo.Reference(remoteRef, true)
			if err != nil {
				return errors.Wrap(err, remoteRef.String())
			}
			fetched = remote.Hash()
			return nil
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		return checkedOut, fetched
	}

	// Clone and check that only the tip commit arrived and is checked out.
	if _, err := conf.CloneOrFetch(ctx, le, ws, sender, timestamp.Now(), nil, nil); err != nil {
		t.Fatal(err.Error())
	}
	if got := storedCommits(); len(got) != 1 || !got[pinned] || got[first] {
		t.Fatalf("clone stored %v, want only %v", got, pinned)
	}
	if checkedOut, fetched := refs(); checkedOut != pinned || fetched != pinned {
		t.Fatalf("after clone: checked out %v, fetched %v, want %v", checkedOut, fetched, pinned)
	}

	// Add two upstream commits and fetch again.
	skipped := commitFile(t, dir, srcWt, "c.txt", "c\n")
	newest := commitFile(t, dir, srcWt, "d.txt", "d\n")
	if _, err := conf.CloneOrFetch(ctx, le, ws, sender, timestamp.Now(), nil, nil); err != nil {
		t.Fatal(err.Error())
	}

	// Check that the fetch added only the new tip and left the worktree pinned.
	got := storedCommits()
	if len(got) != 2 || !got[pinned] || !got[newest] || got[skipped] {
		t.Fatalf("fetch stored %v, want only %v and %v", got, pinned, newest)
	}
	if checkedOut, fetched := refs(); checkedOut != pinned || fetched != newest {
		t.Fatalf("after fetch: checked out %v, fetched %v, want %v and %v", checkedOut, fetched, pinned, newest)
	}
}
