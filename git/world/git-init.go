package git_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// GitInitOpId is the git init operation id.
var GitInitOpId = "hydra/git/init"

// NewGitInitOp constructs a new GitInitOp block.
// repoRef, worktreeArgs can be empty
func NewGitInitOp(
	objKey string,
	repoRef *bucket.ObjectRef,
	disableCheckout bool,
	worktreeArgs *GitCreateWorktreeOp,
) *GitInitOp {
	if disableCheckout {
		worktreeArgs = nil
	}
	return &GitInitOp{
		ObjectKey:       objKey,
		RepoRef:         repoRef,
		DisableCheckout: disableCheckout,
		CreateWorktree:  worktreeArgs,
	}
}

// NewGitInitOpBlock constructs a new GitInitOp block.
func NewGitInitOpBlock() block.Block {
	return &GitInitOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *GitInitOp) GetOperationTypeId() string {
	return GitInitOpId
}

// Validate checks the init operation.
func (o *GitInitOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if !o.GetRepoRef().GetEmpty() {
		if err := o.GetRepoRef().Validate(); err != nil {
			return errors.Wrap(err, "repo_ref")
		}
	}
	if !o.GetDisableCheckout() {
		if err := o.GetCreateWorktree().Validate(); err != nil {
			return errors.Wrap(err, "create_worktree")
		}
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *GitInitOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	repoRef := o.GetRepoRef()

	// create / validate the objectref for the repo
	repoRef, err = ValidateOrCreateRepo(ctx, worldHandle.AccessWorldState, repoRef)
	if err != nil {
		return false, err
	}

	// create the repo object
	_, err = worldHandle.CreateObject(ctx, objKey, repoRef)
	if err != nil {
		return false, err
	}

	// repo type -> types/git/repo
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, GitRepoTypeID); err != nil {
		return false, err
	}

	// if configured perform the checkout
	if !o.GetDisableCheckout() {
		op := o.GetCreateWorktree()
		if op == nil {
			op = &GitCreateWorktreeOp{}
		}
		op.RepoObjectKey = objKey
		_, sysErr, err = worldHandle.ApplyWorldOp(ctx, op, sender)
		if err != nil {
			return sysErr, err
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *GitInitOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Applying to an existing object.
	// Disable checkout, ignore object key.
	repoRef := o.GetRepoRef()

	// create / validate the objectref for the repo
	repoRef, err = ValidateOrCreateRepo(ctx, objectHandle.AccessWorldState, repoRef)
	if err != nil {
		return false, err
	}

	// update the object
	_, err = objectHandle.SetRootRef(ctx, repoRef)
	return false, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *GitInitOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *GitInitOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*GitInitOp)(nil))
