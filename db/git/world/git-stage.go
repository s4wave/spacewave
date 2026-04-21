package git_world

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/world"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// GitStageOpId is the git stage operation id.
var GitStageOpId = "hydra/git/stage"

// GetOperationTypeId returns the operation type identifier.
func (o *GitStageOp) GetOperationTypeId() string {
	return GitStageOpId
}

// Validate checks the stage operation.
func (o *GitStageOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if o.GetRepoObjectKey() == "" {
		return errors.New("repo_object_key is required")
	}
	if len(o.GetPaths()) == 0 {
		return errors.New("paths is required")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *GitStageOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	repoObjKey := o.GetRepoObjectKey()
	ts := o.GetTimestamp().AsTime()
	paths := o.GetPaths()

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
				return errors.Wrap(err, "worktree")
			}
			for _, p := range paths {
				if _, err := wt.Add(p); err != nil {
					return errors.Wrap(err, "stage "+p)
				}
			}
			return nil
		},
	)
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *GitStageOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *GitStageOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *GitStageOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*GitStageOp)(nil))
