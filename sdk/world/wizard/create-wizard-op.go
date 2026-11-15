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

// Validate performs cursory checks on the op.
func (o *CreateWizardObjectOp) Validate() error {
	// Require the wizard object key, type identifiers and valid timestamp.
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

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateWizardObjectOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the wizard creation request before changing World state.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// A wizard reusing a key starts with no ending: the ending an earlier
	// wizard at this key recorded would read as this one's.
	objKey := o.GetObjectKey()
	if err := clearWizardEnding(ctx, worldHandle, objKey); err != nil {
		return false, err
	}

	// Create the World object with the initial wizard step and target configuration.
	state := &WizardState{
		Step:            o.GetInitialStep(),
		TargetTypeId:    o.GetTargetTypeId(),
		TargetKeyPrefix: o.GetTargetKeyPrefix(),
		Name:            o.GetName(),
		ConfigData:      o.GetInitialConfigData(),
	}
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, worldHandle, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Assign the wizard type to the newly created World object.
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, o.GetWizardTypeId()); err != nil {
		return false, err
	}

	return false, nil
}

var _ world.Operation = (*CreateWizardObjectOp)(nil)
