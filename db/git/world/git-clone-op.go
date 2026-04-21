package git_world

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// GitCloneOpId is the git clone operation id.
var GitCloneOpId = "hydra/git/clone"

// NewGitCloneOp constructs a new GitCloneOp.
func NewGitCloneOp(op *GitCloneOp) *GitCloneOp {
	return op
}

// GetOperationTypeId returns the operation type identifier.
func (o *GitCloneOp) GetOperationTypeId() string {
	return GitCloneOpId
}

// Validate checks the clone operation.
func (o *GitCloneOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetCloneOpts().Validate(); err != nil {
		return errors.Wrap(err, "clone_opts")
	}
	if !o.GetDisableCheckout() {
		if wt := o.GetCreateWorktree(); wt != nil {
			if err := wt.Validate(); err != nil {
				return errors.Wrap(err, "create_worktree")
			}
		}
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *GitCloneOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	cloneOpts := o.GetCloneOpts()
	if o.GetDisableCheckout() {
		cloneOpts = cloneOpts.CloneVT()
		cloneOpts.DisableCheckout = true
	}
	_, err = GitClone(
		ctx,
		worldHandle,
		o.GetObjectKey(),
		sender,
		cloneOpts,
		nil,
		nil,
		o.GetCreateWorktree(),
		o.GetTimestamp(),
	)
	return false, err
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *GitCloneOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *GitCloneOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *GitCloneOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*GitCloneOp)(nil))
