package git_world

import (
	"context"
	"io/fs"
	"os"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/pkg/errors"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// materializeRepoToTempWorkdir seeds a temporary worktree from the current
// UnixFS workdir (if provided), runs the git callback against that temp
// filesystem, and returns the temp dir path for later batch import.
func materializeRepoToTempWorkdir(
	ctx context.Context,
	ws world.WorldState,
	repoObjKey string,
	updateWorld bool,
	indexStore storer.IndexStorer,
	refStore git_block.ReferenceStore,
	seedHandle *unixfs.FSHandle,
	cb func(repo *git.Repository, workDir billy.Filesystem) error,
) (string, error) {
	tempDir, err := os.MkdirTemp("", "hydra-git-worktree-*")
	if err != nil {
		return "", err
	}
	tempBfs := osfs.New(tempDir)
	if seedHandle != nil {
		if err := unixfs_sync.SyncToBilly(
			ctx,
			tempBfs,
			seedHandle,
			unixfs_sync.DeleteMode_DeleteMode_DURING,
			nil,
		); err != nil {
			os.RemoveAll(tempDir)
			return "", err
		}
	}
	_, _, err = AccessWorldObjectRepo(
		ctx,
		ws,
		repoObjKey,
		updateWorld,
		indexStore,
		tempBfs,
		refStore,
		func(repo *git.Repository) error {
			if cb == nil {
				return nil
			}
			return cb(repo, tempBfs)
		},
	)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", err
	}
	return tempDir, nil
}

// syncFSToUnixfsRefBatch resets the target UnixFS object, then imports srcFs
// through the batch writer so the resulting workdir tree is written in one
// logical commit.
func syncFSToUnixfsRefBatch(
	ctx context.Context,
	ws world.WorldState,
	workdirRef *unixfs_world.UnixfsRef,
	sender peer.ID,
	ts time.Time,
	srcFs fs.FS,
) error {
	if path := workdirRef.GetPath(); path != nil && len(path.GetNodes()) != 0 {
		return errors.New("batch worktree sync does not support non-root workdir paths")
	}
	_, _, err := unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		workdirRef.GetObjectKey(),
		workdirRef.GetFsType(),
		nil,
		true,
		ts,
	)
	if err != nil {
		return err
	}
	srcCursor, err := unixfs_iofs.NewFSCursor(srcFs)
	if err != nil {
		return err
	}
	srcHandle, err := unixfs.NewFSHandle(srcCursor)
	if err != nil {
		srcCursor.Release()
		return err
	}
	defer srcHandle.Release()

	b := unixfs_world.NewBatchFSWriter(
		ws,
		workdirRef.GetObjectKey(),
		workdirRef.GetFsType(),
		sender,
	)
	defer b.Release()
	return unixfs_sync.SyncToUnixfsBatch(ctx, b, srcHandle, nil)
}

// checkoutRepoWorktree applies checkout semantics without pre-setting HEAD.
// go-git's Worktree.Checkout updates HEAD before Reset, which causes a commit
// checkout to preserve files deleted by the target tree as untracked.
func checkoutRepoWorktree(
	repo *git.Repository,
	opts *git.CheckoutOptions,
) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	if opts.Create {
		return wt.Checkout(opts)
	}

	if opts.Branch == "" && opts.Hash.IsZero() {
		href, err := repo.Head()
		if err != nil {
			return err
		}
		opts.Branch = href.Name()
	}

	resetHash, headRef, err := resolveCheckoutTarget(repo, opts)
	if err != nil {
		return err
	}

	ro := &git.ResetOptions{
		Commit:     resetHash,
		Mode:       git.MergeReset,
		SparseDirs: opts.SparseCheckoutDirectories,
	}
	if opts.Force {
		ro.Mode = git.HardReset
	} else if opts.Keep {
		ro.Mode = git.SoftReset
	}
	if err := wt.Reset(ro); err != nil {
		return err
	}
	if headRef == nil {
		return nil
	}
	return repo.Storer.SetReference(headRef)
}

// resolveCheckoutTarget resolves the reset commit and final HEAD reference for
// a checkout request.
func resolveCheckoutTarget(
	repo *git.Repository,
	opts *git.CheckoutOptions,
) (plumbing.Hash, *plumbing.Reference, error) {
	if !opts.Hash.IsZero() {
		return opts.Hash, nil, nil
	}
	if opts.Branch == "" {
		return plumbing.ZeroHash, nil, errors.New("checkout target empty")
	}

	ref, err := storer.ResolveReference(repo.Storer, opts.Branch)
	if err != nil {
		return plumbing.ZeroHash, nil, err
	}
	return ref.Hash(), plumbing.NewSymbolicReference(plumbing.HEAD, opts.Branch), nil
}
