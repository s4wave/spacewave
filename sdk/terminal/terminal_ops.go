package s4wave_terminal

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
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
		TargetKind:      TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE,
		Cols:            DefaultTerminalCols,
		Rows:            DefaultTerminalRows,
		Timestamp:       timestamppb.New(ts),
	}
}

// NewCreateSshHostTerminalOp constructs a CreateTerminalOp for an SSH Host target.
func NewCreateSshHostTerminalOp(objKey, name, sshHostObjKey string, ts time.Time) *CreateTerminalOp {
	return &CreateTerminalOp{
		ObjectKey:        objKey,
		Name:             name,
		SshHostObjectKey: sshHostObjKey,
		TargetKind:       TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST,
		Cols:             DefaultTerminalCols,
		Rows:             DefaultTerminalRows,
		Timestamp:        timestamppb.New(ts),
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
	switch effectiveCreateTerminalOpTargetKind(o) {
	case TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE:
		if strings.TrimSpace(o.GetDeviceObjectKey()) == "" {
			return world.ErrEmptyOp
		}
		if strings.TrimSpace(o.GetDevicePeerId()) == "" {
			return world.ErrEmptyOp
		}
	case TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST:
		if strings.TrimSpace(o.GetSshHostObjectKey()) == "" {
			return world.ErrEmptyOp
		}
	default:
		return world.ErrEmptyOp
	}
	if err := validateTerminalEnvironment(o.GetEnvironment()); err != nil {
		return err
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if o.GetReconcileExisting() && len(o.GetCreationToken()) < 16 {
		return world.ErrEmptyOp
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

	terminal := o.buildTerminal()
	if o.GetReconcileExisting() {
		existing, objRef, lookupErr := world.LookupObject[*Terminal](ctx, ws, o.GetObjectKey(), NewTerminalBlock)
		world.ReleaseObjectState(objRef)
		if lookupErr == nil {
			if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), TerminalTypeID); err != nil {
				return false, err
			}
			if terminalMatchesDesired(existing, terminal) {
				return false, nil
			}
			return false, world.ErrObjectExists
		}
		if !errors.Is(lookupErr, world.ErrObjectNotFound) {
			return false, lookupErr
		}
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

func (o *CreateTerminalOp) buildTerminal() *Terminal {
	cols, rows := NormalizeTerminalFrameSize(o.GetCols(), o.GetRows())
	return &Terminal{
		Name:             o.GetName(),
		DeviceObjectKey:  o.GetDeviceObjectKey(),
		DevicePeerId:     o.GetDevicePeerId(),
		SshHostObjectKey: o.GetSshHostObjectKey(),
		TargetKind:       effectiveCreateTerminalOpTargetKind(o),
		Command:          o.GetCommand(),
		Environment:      slices.Clone(o.GetEnvironment()),
		Cols:             cols,
		Rows:             rows,
		State:            TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED,
		Status:           "not connected",
		CreatedAt:        o.GetTimestamp(),
		UpdatedAt:        o.GetTimestamp(),
		CreationToken:    slices.Clone(o.GetCreationToken()),
	}
}

func terminalMatchesDesired(existing, desired *Terminal) bool {
	return existing != nil && desired != nil && existing.EqualVT(desired)
}

func effectiveCreateTerminalOpTargetKind(o *CreateTerminalOp) TerminalTargetKind {
	if o == nil {
		return TerminalTargetKind_TERMINAL_TARGET_KIND_UNKNOWN
	}
	switch o.GetTargetKind() {
	case TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE,
		TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST:
		return o.GetTargetKind()
	}
	if strings.TrimSpace(o.GetSshHostObjectKey()) != "" {
		return TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST
	}
	if strings.TrimSpace(o.GetDeviceObjectKey()) != "" || strings.TrimSpace(o.GetDevicePeerId()) != "" {
		return TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE
	}
	return TerminalTargetKind_TERMINAL_TARGET_KIND_UNKNOWN
}

// _ is a type assertion
var _ world.Operation = (*CreateTerminalOp)(nil)
