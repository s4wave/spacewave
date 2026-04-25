package s4wave_wizard

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateWizardObjectOpId is the operation id for CreateWizardObjectOp.
var CreateWizardObjectOpId = "spacewave/wizard/create"

// NewCreateWizardObjectOp constructs a new CreateWizardObjectOp.
func NewCreateWizardObjectOp(
	objKey string,
	wizardTypeId string,
	targetTypeId string,
	targetKeyPrefix string,
	name string,
	ts time.Time,
) *CreateWizardObjectOp {
	return &CreateWizardObjectOp{
		ObjectKey:       objKey,
		WizardTypeId:    wizardTypeId,
		TargetTypeId:    targetTypeId,
		TargetKeyPrefix: targetKeyPrefix,
		Name:            name,
		Timestamp:       timestamp.New(ts),
	}
}

// NewCreateWizardObjectOpBlock constructs a new CreateWizardObjectOp block.
func NewCreateWizardObjectOpBlock() block.Block {
	return &CreateWizardObjectOp{}
}

// Validate performs cursory checks on the op.
func (o *CreateWizardObjectOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(o.GetWizardTypeId()) == 0 {
		return errors.New("wizard type id is required")
	}
	if len(o.GetTargetTypeId()) == 0 {
		return errors.New("target type id is required")
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateWizardObjectOp) GetOperationTypeId() string {
	return CreateWizardObjectOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateWizardObjectOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	state := &WizardState{
		Step:            o.GetInitialStep(),
		TargetTypeId:    o.GetTargetTypeId(),
		TargetKeyPrefix: o.GetTargetKeyPrefix(),
		Name:            o.GetName(),
		ConfigData:      o.GetInitialConfigData(),
	}
	_, _, err = world.CreateWorldObject(ctx, worldHandle, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, worldHandle, objKey, o.GetWizardTypeId()); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateWizardObjectOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateWizardObjectOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CreateWizardObjectOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateWizardObjectOp looks up a CreateWizardObjectOp operation type.
func LookupCreateWizardObjectOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateWizardObjectOpId {
		return &CreateWizardObjectOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CreateWizardObjectOp)(nil))
