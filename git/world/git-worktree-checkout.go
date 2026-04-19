package git_world

import (
	"context"
	"os"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	git_block "github.com/aperturerobotics/hydra/git/block"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/world"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	"github.com/pkg/errors"
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

	var checkoutDir string
	defer func() {
		if checkoutDir != "" {
			_ = os.RemoveAll(checkoutDir)
		}
	}()

	_, _, err = AccessWorldObjectWorktree(
		ctx,
		worldHandle,
		objKey,
		true,
		nil,
		func(bcs *block.Cursor, worktree *Worktree) error {
			bcs.SetBlock(worktree, true)
			hrs, err := worktree.FollowHeadRefStore(bcs)
			if err != nil {
				return err
			}
			checkoutDir, err = materializeRepoToTempWorkdir(
				ctx,
				worldHandle,
				repoObjKey,
				true,
				worktree,
				hrs,
				wdFsHandle,
				func(repo *git.Repository, _ billy.Filesystem) error {
					if err := checkoutRepoWorktree(repo, checkoutOpts); err != nil {
						return err
					}
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
var _ world.Operation = ((*GitWorktreeCheckoutOp)(nil))
