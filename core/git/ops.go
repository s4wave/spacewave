package s4wave_git

import (
	"context"

	"github.com/pkg/errors"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Validate performs cursory validation of the operation.
func (o *CreateGitRepoWizardOp) Validate() error {
	if o.GetObjectKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}
	if o.GetClone() {
		if o.GetCloneOpts() == nil {
			return errors.New("clone_opts required when clone is true")
		}
		if err := o.GetCloneOpts().Validate(); err != nil {
			return errors.Wrap(err, "clone_opts")
		}
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateGitRepoWizardOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Read the target object key from the operation.
	objKey := o.GetObjectKey()

	// Reject clone requests until import support lands.
	if o.GetClone() {
		return false, world.NewOpRefusedError(errors.New("clone must be imported before applying create git repo op"))
	}

	// Build the worktree and init operations for the new repository.
	worktreeOp := &git_world.GitCreateWorktreeOp{
		ObjectKey:       objKey + "/worktree",
		CreateWorkdir:   true,
		WorkdirRef:      &unixfs_world.UnixfsRef{ObjectKey: objKey + "/workdir"},
		DisableCheckout: true,
		Timestamp:       o.GetTimestamp(),
	}
	initOp := git_world.NewGitInitOp(objKey, nil, false, worktreeOp, o.GetTimestamp())
	_, sysErr, err = ws.ApplyWorldOp(ctx, initOp, sender)
	return sysErr, err
}

// NewCreateGitRepoWizardOp constructs an empty CreateGitRepoWizardOp.
//
// GoScript builds defer this package until the first call to this function,
// so schemas there construct the operation through it and match its type ID
// with opids.CreateRepoWizard.
func NewCreateGitRepoWizardOp() any {
	return new(CreateGitRepoWizardOp)
}

// _ is a type assertion
var _ world.Operation = (*CreateGitRepoWizardOp)(nil)
