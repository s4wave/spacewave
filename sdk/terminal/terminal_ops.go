package s4wave_terminal

import (
	"context"
	"slices"
	"strings"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateTerminalOpId is the operation id for CreateTerminalOp.
var CreateTerminalOpId = "spacewave/terminal/create"

// NewCreateTerminalOp constructs a new CreateTerminalOp.
func NewCreateTerminalOp(objKey, name, deviceObjKey, devicePeerID string, ts time.Time) *CreateTerminalOp {
	return &CreateTerminalOp{
		ObjectKey:       objKey,
		Name:            name,
		DeviceObjectKey: deviceObjKey,
		DevicePeerId:    devicePeerID,
		Cols:            DefaultTerminalCols,
		Rows:            DefaultTerminalRows,
		Timestamp:       timestamppb.New(ts),
	}
}

// NewCreateTerminalOpBlock constructs a CreateTerminalOp block.
func NewCreateTerminalOpBlock() block.Block {
	return &CreateTerminalOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateTerminalOp) GetOperationTypeId() string {
	return CreateTerminalOpId
}

// Validate performs cursory checks on the op.
func (o *CreateTerminalOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if strings.TrimSpace(o.GetName()) == "" {
		return world.ErrEmptyOp
	}
	if strings.TrimSpace(o.GetDeviceObjectKey()) == "" {
		return world.ErrEmptyOp
	}
	if strings.TrimSpace(o.GetDevicePeerId()) == "" {
		return world.ErrEmptyOp
	}
	if err := validateTerminalEnvironment(o.GetEnvironment()); err != nil {
		return err
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateTerminalOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	cols, rows := NormalizeTerminalFrameSize(o.GetCols(), o.GetRows())
	terminal := &Terminal{
		Name:            o.GetName(),
		DeviceObjectKey: o.GetDeviceObjectKey(),
		DevicePeerId:    o.GetDevicePeerId(),
		Command:         o.GetCommand(),
		Environment:     slices.Clone(o.GetEnvironment()),
		Cols:            cols,
		Rows:            rows,
		State:           TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED,
		Status:          "not connected",
		CreatedAt:       o.GetTimestamp(),
		UpdatedAt:       o.GetTimestamp(),
	}

	_, _, err = world.CreateWorldObject(ctx, ws, o.GetObjectKey(), func(bcs *block.Cursor) error {
		bcs.SetBlock(terminal, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, o.GetObjectKey(), TerminalTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateTerminalOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateTerminalOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *CreateTerminalOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateTerminalOp looks up a CreateTerminalOp operation type.
func LookupCreateTerminalOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateTerminalOpId {
		return &CreateTerminalOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*CreateTerminalOp)(nil))
