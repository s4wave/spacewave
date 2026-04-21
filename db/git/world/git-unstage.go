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

// GitUnstageOpId is the git unstage operation id.
var GitUnstageOpId = "hydra/git/unstage"

// GetOperationTypeId returns the operation type identifier.
func (o *GitUnstageOp) GetOperationTypeId() string {
	return GitUnstageOpId
}

// Validate checks the unstage operation.
func (o *GitUnstageOp) Validate() error {
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
func (o *GitUnstageOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	repoObjKey := o.GetRepoObjectKey()
	ts := o.GetTimestamp().AsTime()

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

			headRef, err := repo.Head()
			if err != nil {
				return errors.Wrap(err, "head")
			}

			return wt.Reset(&git.ResetOptions{
				Commit: headRef.Hash(),
				Files:  o.GetPaths(),
			})
		},
	)
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *GitUnstageOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *GitUnstageOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *GitUnstageOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*GitUnstageOp)(nil))
