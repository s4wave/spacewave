package git_world

import (
	"context"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket "github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
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
	authMethod client.SSHAuth,
	progress sideband.Progress,
	worktreeArgs *GitCreateWorktreeOp,
	ts *timestamppb.Timestamp,
) (*bucket.ObjectRef, error) {
	cloneArgs := cloneOpts.BuildCloneOpts()
	enableCheckout := !cloneOpts.GetDisableCheckout()

	// Skip checkout during clone. Phase 2 (CreateWorldObjectWorktree)
	// handles the real checkout on the hydra-backed filesystem.
	// Cloning with checkout builds a git index in the Store that leaks
	// into Phase 2 and confuses the MergeReset diff logic.
	cloneArgs.NoCheckout = true
	// write progress if necessary
	cloneArgs.Progress = progress
	// override auth method
	if authMethod != nil {
		cloneArgs.ClientOptions = append(cloneArgs.ClientOptions, client.WithSSHAuth(authMethod))
	}

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
