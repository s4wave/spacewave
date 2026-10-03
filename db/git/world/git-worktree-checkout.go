package git_world

import (
	"context"
	"os"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	git_block "github.com/s4wave/spacewave/db/git/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// GitWorktreeCheckoutOpId is the git init operation id.
var GitWorktreeCheckoutOpId = "hydra/git/worktree/checkout"

// NewGitWorktreeCheckoutOp constructs a new GitWorktreeCheckoutOp block.
// workdirObjKey, workdirPath, and ref can be empty.
func NewGitWorktreeCheckoutOp(
	objKey string,
	repoObjKey string,
	checkoutOpts *git_block.CheckoutOpts,
) *GitWorktreeCheckoutOp {
	return &GitWorktreeCheckoutOp{
		ObjectKey:     objKey,
		RepoObjectKey: repoObjKey,
		CheckoutOpts:  checkoutOpts,
	}
}

// NewGitWorktreeCheckoutOpBlock constructs a new GitWorktreeCheckoutOp block.
func NewGitWorktreeCheckoutOpBlock() block.Block {
	return &GitWorktreeCheckoutOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *GitWorktreeCheckoutOp) GetOperationTypeId() string {
	return GitWorktreeCheckoutOpId
}

// Validate checks the create worktree operation.
func (o *GitWorktreeCheckoutOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetCheckoutOpts().Validate(); err != nil {
		return errors.Wrap(err, "checkout_opts")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *GitWorktreeCheckoutOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Require the worktree and repository object identifiers for checkout.
	objKey := o.GetObjectKey()
	repoObjKey := o.GetRepoObjectKey()
	if objKey == "" || repoObjKey == "" {
		return false, world.ErrEmptyObjectKey
	}

	// call git to checkout to the repo
	ts := o.GetTimestamp().AsTime()
	checkoutOpts, err := o.GetCheckoutOpts().BuildCheckoutOpts()
	if err != nil {
		return false, err
	}

	// Open the referenced UnixFS workdir for checkout materialization.
	workdirRef, err := WorktreeLookupWorkdirRef(ctx, worldHandle, objKey)
	if err != nil {
		return false, err
	}
	wdFsHandle, err := unixfs_world.BuildFSFromUnixfsRef(
		ctx,
		le,
		worldHandle,
		sender,
		workdirRef,
		true,
		false,
		ts,
	)
	if err != nil {
		return false, err
	}
	defer wdFsHandle.Release()

	// Arrange cleanup of the materialized checkout directory.
	var checkoutDir string
	defer func() {
		if checkoutDir != "" {
			_ = os.RemoveAll(checkoutDir)
		}
	}()

	// Check out the repository and persist the worktree index.
	_, _, err = AccessWorldObjectWorktree(
		ctx,
		worldHandle,
		objKey,
		true,
		nil,
		func(bcs *block.Cursor, worktree *Worktree) error {
			// Open the worktree's HEAD reference store before materializing checkout.
			bcs.SetBlock(worktree, true)
			hrs, err := worktree.FollowHeadRefStore(bcs)
			if err != nil {
				return err
			}

			// Materialize the repository checkout and capture its index.
			checkoutDir, err = materializeRepoToTempWorkdir(
				ctx,
				worldHandle,
				repoObjKey,
				true,
				worktree,
				hrs,
				wdFsHandle,
				func(repo *git.Repository, _ billy.Filesystem) error {
					// Check out the requested repository branch or commit.
					if err := checkoutRepoWorktree(repo, checkoutOpts); err != nil {
						return err
					}

					// Persist the checked-out repository index in the worktree.
					idx, err := repo.Storer.Index()
					if err != nil {
						return err
					}
					return worktree.SetIndex(idx)
				},
			)
			return err
		},
	)
	if err != nil {
		return false, err
	}

	// Synchronize the checked-out files into the UnixFS workdir.
	if err := syncFSToUnixfsRefBatch(
		ctx,
		worldHandle,
		workdirRef,
		sender,
		ts,
		os.DirFS(checkoutDir),
	); err != nil {
		return false, err
	}
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *GitWorktreeCheckoutOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *GitWorktreeCheckoutOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *GitWorktreeCheckoutOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = (*GitWorktreeCheckoutOp)(nil)
