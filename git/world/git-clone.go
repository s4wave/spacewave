package git_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	bucket "github.com/aperturerobotics/hydra/bucket"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GitClone performs a git clone operation against a world.
// Clones the repo to the world storage, then submits an op to init it.
// If DisableCheckout is set, disables creating the worktree.
// Returns the object ref to the Repo.
// authMethod, progress, worktreeArgs can be empty.
func GitClone(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	cloneOpts *git_block.CloneOpts,
	authMethod transport.AuthMethod,
	progress sideband.Progress,
	objKey string,
	worktreeArgs *GitCreateWorktreeOp,
) (*bucket.ObjectRef, error) {
	cloneArgs := cloneOpts.BuildCloneOpts()
	enableCheckout := !cloneOpts.GetDisableCheckout()

	// we need to check out to recurse submodules
	// go-git could be adjusted to remove this requirement
	// see go-git/v5/repository.go:844
	cloneArgs.NoCheckout = false
	// write progress if necessary
	cloneArgs.Progress = progress
	// override auth method
	cloneArgs.Auth = authMethod

	// use an in-memory store for the temporary initial index
	var indexStore storer.IndexStorer = &memory.IndexStorage{}

	// use in-memory worktree for the clone operation
	tmpWorktree := memfs.New()

	// perform the clone operation
	repoRef, err := world.AccessObject(
		ctx,
		ws.AccessWorldState,
		nil,
		func(bcs *block.Cursor) error {
			root := git_block.NewRepo()
			bcs.SetBlock(root, true)
			store, err := git_block.NewStore(ctx, nil, bcs, indexStore)
			if err != nil {
				return err
			}
			defer store.Close()
			_, err = git.CloneContext(ctx, store, tmpWorktree, cloneArgs)
			if err != nil {
				return err
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	// we cloned the repo to repoRef, now create repo and worktree
	initOp := NewGitInitOp(objKey, repoRef, !enableCheckout, worktreeArgs)
	_, _, err = ws.ApplyWorldOp(initOp, sender)
	if err != nil {
		return nil, err
	}
	return repoRef, nil
}
