package s4wave_git

import (
	"context"

	"github.com/pkg/errors"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateGitRepoWizardOpId is the operation id for CreateGitRepoWizardOp.
var CreateGitRepoWizardOpId = "spacewave/git/repo/create"

// GetOperationTypeId returns the operation type identifier.
func (o *CreateGitRepoWizardOp) GetOperationTypeId() string {
	return CreateGitRepoWizardOpId
}

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
	objKey := o.GetObjectKey()

	if o.GetClone() {
		return false, errors.New("clone must be imported before applying create git repo op")
	}

	initOp := git_world.NewGitInitOp(objKey, nil, true, nil, o.GetTimestamp())
	_, sysErr, err = ws.ApplyWorldOp(ctx, initOp, sender)
	return sysErr, err
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateGitRepoWizardOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateGitRepoWizardOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CreateGitRepoWizardOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateGitRepoWizardOp looks up a CreateGitRepoWizardOp operation type.
func LookupCreateGitRepoWizardOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateGitRepoWizardOpId {
		return &CreateGitRepoWizardOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CreateGitRepoWizardOp)(nil))
