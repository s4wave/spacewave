package s4wave_vm

import (
	"context"
	"time"

	"github.com/aperturerobotics/cayley/quad"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewCreateWasiVmOp constructs a new CreateWasiVmOp. rootfsObjectKey points at
// the UnixFS object holding the root filesystem and imageObjectKey at the
// WasiVmImage supplying it; at least one is required.
func NewCreateWasiVmOp(objKey, name, rootfsObjectKey, imageObjectKey string, ts time.Time) *CreateWasiVmOp {
	return &CreateWasiVmOp{
		ObjectKey:       objKey,
		Name:            name,
		RootfsObjectKey: rootfsObjectKey,
		ImageObjectKey:  imageObjectKey,
		Timestamp:       timestamppb.New(ts),
	}
}

// Validate performs cursory checks on the op.
func (o *CreateWasiVmOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if len(o.GetRootfsObjectKey()) == 0 && len(o.GetImageObjectKey()) == 0 {
		return errors.New("rootfs_object_key or image_object_key is required")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateWasiVmOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the VM creation intent before changing World state.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create the VM object with its initial metadata block.
	objKey := o.GetObjectKey()
	vm := &WasiVm{
		Name:      o.GetName(),
		Config:    o.GetConfig(),
		CreatedAt: o.GetTimestamp(),
	}
	createdObject, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(vm, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Mark the object type and link the root filesystem and image.
	if err := world_types.SetObjectType(ctx, ws, objKey, WasiVmTypeID); err != nil {
		return false, err
	}
	for _, edge := range []struct {
		pred   quad.IRI
		target string
	}{
		{PredWasiVmRootfs, o.GetRootfsObjectKey()},
		{PredWasiVmImage, o.GetImageObjectKey()},
	} {
		if edge.target == "" {
			continue
		}
		q := world.NewGraphQuadWithKeys(objKey, string(edge.pred), edge.target, "")
		if err := ws.SetGraphQuad(ctx, q); err != nil {
			return true, errors.Wrapf(err, "set %s edge", edge.pred)
		}
	}
	return false, nil
}

// NewSetWasiVmStateOp constructs a new SetWasiVmStateOp.
func NewSetWasiVmStateOp(objKey string, state VmState) *SetWasiVmStateOp {
	return &SetWasiVmStateOp{ObjectKey: objKey, State: state}
}

// Validate performs cursory checks on the op.
func (o *SetWasiVmStateOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	_, err := normalizeDesiredState(o.GetState())
	return err
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetWasiVmStateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the desired state and resolve the typed VM object.
	if err := o.Validate(); err != nil {
		return false, err
	}
	objState, sysErr, err := resolveVmObject(ctx, ws, o.GetObjectKey(), WasiVmTypeID)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return sysErr, err
	}

	// Begin a new run generation toward the requested state.
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		// Decode the VM block.
		vm, unmarshalErr := block.UnmarshalBlock[WasiVm](ctx, bcs)
		if unmarshalErr != nil {
			return world.RefuseDecodeError(unmarshalErr)
		}
		if vm == nil {
			return world.NewOpRefusedError(errors.New("wasivm block missing on object"))
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

// _ is a type assertion
var (
	_ world.Operation = (*CreateWasiVmOp)(nil)
	_ world.Operation = (*SetWasiVmStateOp)(nil)
)
