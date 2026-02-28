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
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/pkg/errors"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GitClone performs a git clone operation against a world.
// Clones the repo to in-memory storage first (no world lock), then copies
// objects to world storage under a brief write lock.
// If DisableCheckout is set, disables creating the worktree.
// Returns the object ref to the Repo.
// authMethod, progress, worktreeArgs can be empty.
func GitClone(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	sender peer.ID,
	cloneOpts *git_block.CloneOpts,
	authMethod transport.AuthMethod,
	progress sideband.Progress,
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

	// Phase 1: clone into in-memory storage (no world lock held).
	// This performs all network I/O and packfile delta resolution in memory.
	memStore := memory.NewStorage()
	tmpWorktree := memfs.New()
	_, err := git.CloneContext(ctx, memStore, tmpWorktree, cloneArgs)
	if err != nil {
		return nil, errors.Wrap(err, "clone to memory")
	}

	// Phase 2: copy from memory to hydra storage under world write lock.
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
			if err := copyMemStoreToHydra(ctx, memStore, store); err != nil {
				return errors.Wrap(err, "copy to hydra")
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	// we cloned the repo to repoRef, now create repo and worktree
	initOp := NewGitInitOp(objKey, repoRef, !enableCheckout, worktreeArgs)
	_, _, err = ws.ApplyWorldOp(ctx, initOp, sender)
	if err != nil {
		return nil, err
	}
	return repoRef, nil
}

// copyMemStoreToHydra copies all git objects, references, and submodules
// from an in-memory storage to a hydra git block store.
func copyMemStoreToHydra(ctx context.Context, src *memory.Storage, dst *git_block.Store) error {
	// copy encoded objects
	iter, err := src.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return err
	}
	if err := iter.ForEach(func(obj plumbing.EncodedObject) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := dst.SetEncodedObject(obj)
		return err
	}); err != nil {
		return err
	}

	// copy references
	refIter, err := src.IterReferences()
	if err != nil {
		return err
	}
	if err := refIter.ForEach(func(ref *plumbing.Reference) error {
		return dst.SetReference(ref)
	}); err != nil {
		return err
	}

	// copy submodules
	for name, subStore := range src.ModuleStorage {
		subStorer, err := dst.Module(name)
		if err != nil {
			return errors.Wrapf(err, "create submodule %s", name)
		}
		subDst, ok := subStorer.(*git_block.Store)
		if !ok {
			continue
		}
		if err := copyMemStoreToHydra(ctx, subStore, subDst); err != nil {
			return errors.Wrapf(err, "copy submodule %s", name)
		}
	}

	return nil
}

