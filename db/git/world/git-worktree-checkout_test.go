package git_world

import (
	"os"
	"testing"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/util"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
)

// TestCheckoutRepoWorktreeHashDetachesHEAD checks the checkout transition from
// a symbolic branch HEAD to a detached hash HEAD, including later commits.
func TestCheckoutRepoWorktreeHashDetachesHEAD(t *testing.T) {
	// Open an in-memory repository with a symbolic branch HEAD.
	fs := memfs.New()
	repo, err := git.Init(memory.NewStorage(), git.WithWorkTree(fs))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Error(err)
		}
	})
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	// Create the base commit that the detached checkout will select.
	if err := util.WriteFile(fs, "base.txt", []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("base.txt"); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@example.com"}
	base, err := wt.Commit("base", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatal(err)
	}

	// Advance the branch, then check out the earlier base as a detached HEAD.
	if err := util.WriteFile(fs, "later.txt", []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("later.txt"); err != nil {
		t.Fatal(err)
	}
	tip, err := wt.Commit("later", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkoutRepoWorktree(repo, &git.CheckoutOptions{Hash: base, Force: true}); err != nil {
		t.Fatal(err)
	}

	// Verify the raw and resolved HEAD are detached and the tree is reset.
	head, err := repo.Storer.Reference(plumbing.HEAD)
	if err != nil {
		t.Fatal(err)
	}
	if head.Type() != plumbing.HashReference || head.Hash() != base {
		t.Fatalf("HEAD = %s, want detached hash %s", head, base)
	}
	if _, err := fs.Stat("later.txt"); !os.IsNotExist(err) {
		t.Fatalf("later.txt survived checkout: %v", err)
	}
	resolved, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name() != plumbing.HEAD || resolved.Hash() != base {
		t.Fatalf("resolved HEAD = %s, want detached hash %s", resolved, base)
	}

	// A detached commit must advance HEAD without advancing the old branch.
	if err := util.WriteFile(fs, "child.txt", []byte("child\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("child.txt"); err != nil {
		t.Fatal(err)
	}
	child, err := wt.Commit("child", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the new commit advances only the detached HEAD.
	head, err = repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if head.Name() != plumbing.HEAD || head.Hash() != child {
		t.Fatalf("HEAD after commit = %s, want detached hash %s", head, child)
	}
	branchTip, err := repo.Reference(branch.Name(), true)
	if err != nil {
		t.Fatal(err)
	}
	if branchTip.Hash() != tip {
		t.Fatalf("branch moved to %s, want %s", branchTip.Hash(), tip)
	}
}
