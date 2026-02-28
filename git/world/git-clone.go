package git_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	bucket "github.com/aperturerobotics/hydra/bucket"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/aperturerobotics/hydra/world"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/pkg/errors"
)

// GitClone performs a git clone operation against a world.
// Clones the repo directly into a bulk-mode hydra Store. Objects stream
// to KV via per-object mini-transactions during the clone, and the IAVL
// tree is built bottom-up at commit time.
// If DisableCheckout is set, disables creating the worktree.
// Returns the object ref to the Repo.
// authMethod, progress, worktreeArgs, ts can be empty.
func GitClone(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	sender peer.ID,
	cloneOpts *git_block.CloneOpts,
	authMethod transport.AuthMethod,
	progress sideband.Progress,
	worktreeArgs *GitCreateWorktreeOp,
	ts *timestamppb.Timestamp,
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

	// Clone directly into hydra storage under world write lock.
	// The bulk-mode Store streams objects to KV via mini-transactions,
	// then builds the IAVL tree bottom-up at Commit.
	repoRef, err := world.AccessObject(
		ctx,
		ws.AccessWorldState,
		nil,
		func(bcs *block.Cursor) error {
			root := git_block.NewRepo()
			bcs.SetBlock(root, true)
			store, err := git_block.NewStore(ctx, nil, bcs, &memory.IndexStorage{}, nil)
			if err != nil {
				return err
			}
			defer store.Close()

			worktree := memfs.New()
			_, err = git.CloneContext(ctx, store, worktree, cloneArgs)
			if err != nil {
				return errors.Wrap(err, "clone")
			}

			return store.Commit()
		},
	)
	if err != nil {
		return nil, err
	}

	// we cloned the repo to repoRef, now create repo and worktree
	initOp := NewGitInitOp(objKey, repoRef, !enableCheckout, worktreeArgs, ts)
	_, _, err = ws.ApplyWorldOp(ctx, initOp, sender)
	if err != nil {
		return nil, err
	}
	return repoRef, nil
}
