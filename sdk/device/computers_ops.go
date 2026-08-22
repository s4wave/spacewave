package s4wave_device

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateComputersDashboardOpId is the operation id for CreateComputersDashboardOp.
var CreateComputersDashboardOpId = "spacewave/computers/create"

// NewCreateComputersDashboardOp constructs a new CreateComputersDashboardOp.
func NewCreateComputersDashboardOp(objKey, name string, ts time.Time) *CreateComputersDashboardOp {
	return &CreateComputersDashboardOp{
		ObjectKey: objKey,
		Name:      name,
		Timestamp: timestamppb.New(ts),
	}
}

// NewCreateComputersDashboardOpBlock constructs a CreateComputersDashboardOp block.
func NewCreateComputersDashboardOpBlock() block.Block {
	return &CreateComputersDashboardOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateComputersDashboardOp) GetOperationTypeId() string {
	return CreateComputersDashboardOpId
}

// Validate performs cursory checks on the op.
func (o *CreateComputersDashboardOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateComputersDashboardOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	dashboard := &ComputersDashboard{
		Name:      o.GetName(),
		CreatedAt: o.GetTimestamp(),
	}

	_, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(dashboard, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, objKey, ComputersDashboardTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateComputersDashboardOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateComputersDashboardOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *CreateComputersDashboardOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateComputersDashboardOp looks up a CreateComputersDashboardOp operation type.
func LookupCreateComputersDashboardOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateComputersDashboardOpId {
		return &CreateComputersDashboardOp{}, nil
	}
	return nil, nil
}

var _ world.Operation = (*CreateComputersDashboardOp)(nil)
