package git_world

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
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
	ts := o.GetTimestamp().ToTime()
	checkoutOpts, err := o.GetCheckoutOpts().BuildCheckoutOpts()
	if err != nil {
		return false, err
	}

	return false, AccessWorldObjectRepoWithWorktree(
		ctx,
		le,
		worldHandle,
		repoObjKey, objKey,
		ts, true,
		sender,
		func(repo *git.Repository, workDir billy.Filesystem) error {
			wt, err := repo.Worktree()
			if err != nil {
				return err
			}

			if checkoutOpts.Branch == "" && checkoutOpts.Hash.IsZero() {
				// checkout the HEAD
				href, err := repo.Head()
				if err != nil {
					return err
				}
				checkoutOpts.Branch = href.Name()
			}

			return wt.Checkout(checkoutOpts)
		},
	)
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
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *GitWorktreeCheckoutOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
}

// _ is a type assertion
var _ world.Operation = ((*GitWorktreeCheckoutOp)(nil))
