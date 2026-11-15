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
	// Validate the VM creation intent before changing World state.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Prepare the VM metadata with its creation timestamp.
	objKey := o.GetObjectKey()
	vm := &VmV86{
		Name:      o.GetName(),
		Config:    o.GetConfig(),
		CreatedAt: o.GetTimestamp(),
	}

	// Create the VM object with its initial metadata block.
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(vm, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Mark the new VM object with its block type.
	if err := world_types.SetObjectType(ctx, ws, objKey, VmV86TypeID); err != nil {
		return false, err
	}

	// Link the VM to its image and configured asset overrides.
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

// NewSetV86ConfigOp constructs a new SetV86ConfigOp.
func NewSetV86ConfigOp(objKey string, config *V86Config) *SetV86ConfigOp {
	return &SetV86ConfigOp{
		ObjectKey: objKey,
		Config:    config,
	}
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

// resolveVmObject returns the object state for a VM key, rejecting missing
// objects and types other than typeID. sysErr reports transport-level
// failures.
func resolveVmObject(ctx context.Context, ws world.WorldState, objKey, typeID string) (objState world.ObjectState, sysErr bool, err error) {
	// Open the requested VM object and release failed lookups.
	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, true, err
	}
	if !found {
		world.ReleaseObjectState(objState)
		return nil, false, world.NewOpRefusedError(errors.Errorf("%s object not found", typeID))
	}

	// Require the opened object to carry the VM block type.
	gotTypeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, true, err
	}
	if gotTypeID != typeID {
		world.ReleaseObjectState(objState)
		return nil, false, world.NewOpRefusedError(errors.Errorf("object %q is not a %s (type=%q)", objKey, typeID, gotTypeID))
	}
	return objState, false, nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetV86ConfigOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the VM configuration update.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Resolve the typed VM object for the configuration change.
	objState, sysErr, err := resolveVmObject(ctx, ws, o.GetObjectKey(), VmV86TypeID)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return sysErr, err
	}

	// Replace the VM's configuration in its retained block.
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		// Decode the VM block and require its retained metadata.
		vm, unmarshalErr := block.UnmarshalBlock[VmV86](ctx, bcs)
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm == nil {
			return world.NewOpRefusedError(errors.New("vm-v86 block missing on object"))
		}

		// Retain the requested configuration in the VM block.
		vm.Config = o.GetConfig()
		bcs.SetBlock(vm, true)
		return nil
	})
	if err != nil {
		return !world.IsOpRefusedError(err), err
	}

	return false, nil
}

// NewSetV86StateOp constructs a new SetV86StateOp.
func NewSetV86StateOp(objKey string, state VmState, errorMessage string) *SetV86StateOp {
	return &SetV86StateOp{
		ObjectKey:    objKey,
		State:        state,
		ErrorMessage: errorMessage,
	}
}

// Validate performs cursory checks on the op.
func (o *SetV86StateOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	_, err := normalizeDesiredState(o.GetState())
	return err
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetV86StateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the desired VM state update.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Resolve the typed VM object for the desired state change.
	objState, sysErr, err := resolveVmObject(ctx, ws, o.GetObjectKey(), VmV86TypeID)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return sysErr, err
	}

	// Begin a new run generation toward the requested state.
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		// Decode the VM block and require its retained execution state.
		vm, unmarshalErr := block.UnmarshalBlock[VmV86](ctx, bcs)
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm == nil {
			return world.NewOpRefusedError(errors.New("vm-v86 block missing on object"))
		}

		// Advance the run generation with its new desired and observed states.
		desired, observed, changed, stateErr := nextRunStates(vm.GetState(), vm.GetObservedState(), o.GetState())
		if stateErr != nil || !changed {
			return world.NewOpRefusedError(stateErr)
		}
		vm.State = desired
		vm.ObservedState = observed
		vm.RunGeneration++
		vm.ErrorMessage = ""
		bcs.SetBlock(vm, true)
		return nil
	})
	if err != nil {
		return !world.IsOpRefusedError(err), err
	}

	return false, nil
}

// _ is a type assertion.
var (
	_ world.Operation = (*CreateVmV86Op)(nil)
	_ world.Operation = (*SetV86ConfigOp)(nil)
	_ world.Operation = (*SetV86StateOp)(nil)
)
