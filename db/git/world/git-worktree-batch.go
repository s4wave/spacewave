package git_world

import (
	"context"
	"io/fs"
	"os"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
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
	// Seed a disposable filesystem from the existing World workdir.
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
			unixfs_sync.DeleteMode_DeleteMode_NONE,
			nil,
		); err != nil {
			os.RemoveAll(tempDir)
			return "", err
		}
	}

	// Apply the Git operation against the temporary files and World stores.
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

	// Transfer the temporary directory to the caller for batch import.
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
	// Reset the root workdir before importing its replacement tree.
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

	// Retain a source handle until the batch writer finishes importing.
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

	// Commit the replacement filesystem through the World batch writer.
	b := unixfs_world.NewBatchFSWriter(
		ws,
		workdirRef.GetObjectKey(),
		workdirRef.GetFsType(),
		sender,
	)
	defer b.Release()
	return unixfs_sync.SyncToUnixfsBatch(ctx, b, srcHandle, nil)
}

// checkoutRepoWorktree uses go-git's checkout contract to update HEAD and the
// worktree together. A sentinel prevents pruning the temporary worktree root.
func checkoutRepoWorktree(repo *git.Repository, opts *git.CheckoutOptions) (err error) {
	// Open the worktree before reserving a file in its filesystem.
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Keep the temporary root non-empty while a forced checkout deletes files.
	if opts.Force {
		worktreeFS := wt.Filesystem()
		sentinel, err := worktreeFS.TempFile(".", ".spacewave-checkout-sentinel-")
		if err != nil {
			return err
		}
		defer func() {
			if removeErr := worktreeFS.Remove(sentinel.Name()); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
				err = removeErr
			}
		}()
		if err := sentinel.Close(); err != nil {
			return err
		}
	}

	// Checkout retains the previous tree for deletions and sets a detached HEAD
	// for a hash target without moving the original branch.
	return wt.Checkout(opts)
}
