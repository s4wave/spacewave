package s4wave_vm

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateVmV86OpId is the operation id for CreateVmV86Op.
var CreateVmV86OpId = "vm/v86/create"

// NewCreateVmV86Op constructs a new CreateVmV86Op. imageObjectKey points at
// the V86Image that supplies default asset edges and is required.
func NewCreateVmV86Op(objKey, name, imageObjectKey string, ts time.Time) *CreateVmV86Op {
	return &CreateVmV86Op{
		ObjectKey:      objKey,
		Name:           name,
		ImageObjectKey: imageObjectKey,
		Timestamp:      timestamppb.New(ts),
	}
}

// NewCreateVmV86OpBlock constructs a new CreateVmV86Op block.
func NewCreateVmV86OpBlock() block.Block {
	return &CreateVmV86Op{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateVmV86Op) GetOperationTypeId() string {
	return CreateVmV86OpId
}

// Validate performs cursory checks on the op.
func (o *CreateVmV86Op) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if len(o.GetImageObjectKey()) == 0 {
		return errors.New("image_object_key is required")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateVmV86Op) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	vm := &VmV86{
		Name:      o.GetName(),
		Config:    o.GetConfig(),
		CreatedAt: o.GetTimestamp(),
	}

	_, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(vm, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, objKey, VmV86TypeID); err != nil {
		return false, err
	}

	edges := []struct {
		pred   string
		target string
	}{
		{string(PredV86Image), o.GetImageObjectKey()},
		{string(PredV86KernelOverride), o.GetKernelOverrideObjectKey()},
		{string(PredV86RootfsOverride), o.GetRootfsOverrideObjectKey()},
		{string(PredV86BiosOverride), o.GetBiosOverrideObjectKey()},
		{string(PredV86WasmOverride), o.GetWasmOverrideObjectKey()},
	}
	for _, e := range edges {
		if e.target == "" {
			continue
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(objKey, e.pred, e.target, "")); err != nil {
			return true, errors.Wrapf(err, "set %s edge", e.pred)
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateVmV86Op) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateVmV86Op) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CreateVmV86Op) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateVmV86Op looks up a CreateVmV86Op operation type.
func LookupCreateVmV86Op(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateVmV86OpId {
		return &CreateVmV86Op{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*CreateVmV86Op)(nil)

// SetV86ConfigOpId is the operation id for SetV86ConfigOp.
var SetV86ConfigOpId = "vm/v86/set-config"

// NewSetV86ConfigOp constructs a new SetV86ConfigOp.
func NewSetV86ConfigOp(objKey string, config *V86Config) *SetV86ConfigOp {
	return &SetV86ConfigOp{
		ObjectKey: objKey,
		Config:    config,
	}
}

// NewSetV86ConfigOpBlock constructs a new SetV86ConfigOp block.
func NewSetV86ConfigOpBlock() block.Block {
	return &SetV86ConfigOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SetV86ConfigOp) GetOperationTypeId() string {
	return SetV86ConfigOpId
}

// Validate performs cursory checks on the op.
func (o *SetV86ConfigOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if o.GetConfig() == nil {
		return errors.New("config is required")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetV86ConfigOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return true, err
	}
	if !found {
		return false, errors.New("vm-v86 object not found")
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return true, err
	}
	if typeID != VmV86TypeID {
		return false, errors.Errorf("object %q is not a VmV86 (type=%q)", objKey, typeID)
	}

	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		vm, unmarshalErr := block.UnmarshalBlock[*VmV86](ctx, bcs, func() block.Block {
			return &VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm == nil {
			return errors.New("vm-v86 block missing on object")
		}
		vm.Config = o.GetConfig()
		bcs.SetBlock(vm, true)
		return nil
	})
	if err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *SetV86ConfigOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *SetV86ConfigOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *SetV86ConfigOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSetV86ConfigOp looks up a SetV86ConfigOp operation type.
func LookupSetV86ConfigOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == SetV86ConfigOpId {
		return &SetV86ConfigOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*SetV86ConfigOp)(nil)

// SetV86StateOpId is the operation id for SetV86StateOp.
var SetV86StateOpId = "vm/v86/set-state"

// IsValidV86StateTransition reports whether transitioning a VmV86 from src to
// dst is permitted by the legacy state graph.
//
// New state operations store desired state in VmV86.State and runtime
// observations in VmV86.ObservedState. This helper remains for callers that
// validate the pre-fenced graph.
func IsValidV86StateTransition(src, dst VmState) bool {
	if src == dst {
		return false
	}
	// any -> ERROR is always allowed.
	if dst == VmState_VmState_ERROR {
		return true
	}
	// ERROR -> STOPPED clears the error.
	if src == VmState_VmState_ERROR {
		return dst == VmState_VmState_STOPPED
	}
	switch src {
	case VmState_VmState_STOPPED:
		return dst == VmState_VmState_STARTING
	case VmState_VmState_STARTING:
		return dst == VmState_VmState_RUNNING || dst == VmState_VmState_STOPPED
	case VmState_VmState_RUNNING:
		return dst == VmState_VmState_STOPPING || dst == VmState_VmState_STOPPED
	case VmState_VmState_STOPPING:
		return dst == VmState_VmState_STOPPED
	}
	return false
}

func normalizeV86DesiredState(state VmState) (VmState, error) {
	switch state {
	case VmState_VmState_STARTING:
		return VmState_VmState_RUNNING, nil
	case VmState_VmState_STOPPING:
		return VmState_VmState_STOPPED, nil
	case VmState_VmState_STOPPED, VmState_VmState_RUNNING:
		return state, nil
	default:
		return VmState_VmState_STOPPED, errors.Errorf("invalid desired v86 state %s", state.String())
	}
}

// NewSetV86StateOp constructs a new SetV86StateOp.
func NewSetV86StateOp(objKey string, state VmState, errorMessage string) *SetV86StateOp {
	return &SetV86StateOp{
		ObjectKey:    objKey,
		State:        state,
		ErrorMessage: errorMessage,
	}
}

// NewSetV86StateOpBlock constructs a new SetV86StateOp block.
func NewSetV86StateOpBlock() block.Block {
	return &SetV86StateOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SetV86StateOp) GetOperationTypeId() string {
	return SetV86StateOpId
}

// Validate performs cursory checks on the op.
func (o *SetV86StateOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetV86StateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return true, err
	}
	if !found {
		return false, errors.New("vm-v86 object not found")
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return true, err
	}
	if typeID != VmV86TypeID {
		return false, errors.Errorf("object %q is not a VmV86 (type=%q)", objKey, typeID)
	}

	target, err := normalizeV86DesiredState(o.GetState())
	if err != nil {
		return false, err
	}
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		vm, unmarshalErr := block.UnmarshalBlock[*VmV86](ctx, bcs, func() block.Block {
			return &VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm == nil {
			return errors.New("vm-v86 block missing on object")
		}

		current := vm.GetState()
		if current == VmState_VmState_ERROR {
			current = VmState_VmState_STOPPED
		} else if normalized, normalizeErr := normalizeV86DesiredState(current); normalizeErr == nil {
			current = normalized
		}
		observed := vm.GetObservedState()
		if target == current &&
			(target != VmState_VmState_RUNNING || observed != VmState_VmState_ERROR) &&
			(target != VmState_VmState_STOPPED || observed == VmState_VmState_STOPPED) {
			return errors.Errorf("v86 desired state already %s", target.String())
		}

		vm.State = target
		vm.RunGeneration++
		vm.ErrorMessage = ""
		if target == VmState_VmState_RUNNING {
			vm.ObservedState = VmState_VmState_STARTING
		} else if observed != VmState_VmState_STOPPED {
			vm.ObservedState = VmState_VmState_STOPPING
		} else {
			vm.ObservedState = VmState_VmState_STOPPED
		}
		bcs.SetBlock(vm, true)
		return nil
	})
	if err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *SetV86StateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *SetV86StateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *SetV86StateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSetV86StateOp looks up a SetV86StateOp operation type.
func LookupSetV86StateOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == SetV86StateOpId {
		return &SetV86StateOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*SetV86StateOp)(nil)
