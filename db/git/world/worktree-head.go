package git_world

import (
	"context"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// LookupWorktreeHead returns the checked-out reference of the worktree object
// at worktreeKey, resolved to a commit in the repository object at repoKey.
// The reference is named by the checked-out branch, or HEAD when detached.
// Returns nil when the worktree has no commit checked out.
func LookupWorktreeHead(ctx context.Context, ws world.WorldState, worktreeKey, repoKey string) (*plumbing.Reference, error) {
	// Read HEAD from the worktree's ref store.
	var head *plumbing.Reference
	_, _, err := AccessWorldObjectWorktree(ctx, ws, worktreeKey, false, nil, func(bcs *block.Cursor, wt *Worktree) error {
		// Follow the ref store and read HEAD when it is set.
		hrs, err := wt.FollowHeadRefStore(bcs)
		if err != nil {
			return err
		}
		head, err = hrs.GetReference(plumbing.HEAD)
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil
		}
		return err
	})
	if err != nil || head == nil {
		return nil, err
	}

	// Resolve a branch HEAD to its commit in the repository.
	if head.Type() == plumbing.SymbolicReference {
		branch := head.Target()
		head = nil
		_, _, err = AccessWorldObjectRepo(ctx, ws, repoKey, false, nil, nil, nil, func(repo *git.Repository) error {
			// Resolve the branch when the repository has it.
			ref, err := repo.Reference(branch, true)
			if errors.Is(err, plumbing.ErrReferenceNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			head = plumbing.NewHashReference(branch, ref.Hash())
			return nil
		})
		if err != nil || head == nil {
			return nil, err
		}
	}

	// Treat a zero hash as no checked-out commit.
	if head.Hash() == plumbing.ZeroHash {
		return nil, nil
	}
	return head, nil
}
