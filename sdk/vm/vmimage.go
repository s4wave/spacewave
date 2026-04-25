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

// VmImageTypeID is the type identifier for VmImage objects.
const VmImageTypeID = "spacewave/vm/image"

// PredVmImageWasm is the graph predicate for the emulator WASM binary.
var PredVmImageWasm = quad.IRI("vmimage/wasm")

// PredVmImageBiosSeabios is the graph predicate for the SeaBIOS asset.
var PredVmImageBiosSeabios = quad.IRI("vmimage/bios/seabios")

// PredVmImageBiosVgabios is the graph predicate for the VGA BIOS asset.
var PredVmImageBiosVgabios = quad.IRI("vmimage/bios/vgabios")

// PredVmImageKernel is the graph predicate for the kernel image asset.
var PredVmImageKernel = quad.IRI("vmimage/kernel")

// PredVmImageRootfs is the graph predicate for the rootfs archive asset.
var PredVmImageRootfs = quad.IRI("vmimage/rootfs")

// GetBlockTypeId returns the block type identifier.
func (v *VmImage) GetBlockTypeId() string {
	return VmImageTypeID
}

// MarshalBlock marshals the block to binary.
func (v *VmImage) MarshalBlock() ([]byte, error) {
	return v.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (v *VmImage) UnmarshalBlock(data []byte) error {
	return v.UnmarshalVT(data)
}

// Validate performs cursory checks on the VmImage.
func (v *VmImage) Validate() error {
	if v.GetName() == "" {
		return errors.New("name is required")
	}
	if v.GetPlatform() == "" {
		return errors.New("platform is required")
	}
	return nil
}

// CreateVmImageOpId is the operation id for CreateVmImageOp.
var CreateVmImageOpId = "spacewave/vm/image/create"

// NewCreateVmImageOp constructs a new CreateVmImageOp.
func NewCreateVmImageOp(objKey string, img *VmImage, ts time.Time) *CreateVmImageOp {
	return &CreateVmImageOp{
		ObjectKey: objKey,
		Image:     img,
		Timestamp: timestamppb.New(ts),
	}
}

// NewCreateVmImageOpBlock constructs a new CreateVmImageOp block.
func NewCreateVmImageOpBlock() block.Block {
	return &CreateVmImageOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateVmImageOp) GetOperationTypeId() string {
	return CreateVmImageOpId
}

// Validate performs cursory checks on the op.
func (o *CreateVmImageOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if o.GetImage() == nil {
		return errors.New("image is required")
	}
	return o.GetImage().Validate()
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateVmImageOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	img := o.GetImage().CloneVT()
	img.CreatedAt = o.GetTimestamp()

	_, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(img, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, objKey, VmImageTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateVmImageOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateVmImageOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CreateVmImageOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateVmImageOp looks up a CreateVmImageOp operation type.
func LookupCreateVmImageOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateVmImageOpId {
		return &CreateVmImageOp{}, nil
	}
	return nil, nil
}

// SetVmImageMetadataOpId is the operation id for SetVmImageMetadataOp.
var SetVmImageMetadataOpId = "spacewave/vm/image/set-metadata"

// NewSetVmImageMetadataOp constructs a new SetVmImageMetadataOp.
func NewSetVmImageMetadataOp(objKey string, img *VmImage) *SetVmImageMetadataOp {
	return &SetVmImageMetadataOp{
		ObjectKey: objKey,
		Image:     img,
	}
}

// NewSetVmImageMetadataOpBlock constructs a new SetVmImageMetadataOp block.
func NewSetVmImageMetadataOpBlock() block.Block {
	return &SetVmImageMetadataOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SetVmImageMetadataOp) GetOperationTypeId() string {
	return SetVmImageMetadataOpId
}

// Validate performs cursory checks on the op.
func (o *SetVmImageMetadataOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if o.GetImage() == nil {
		return errors.New("image is required")
	}
	return o.GetImage().Validate()
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetVmImageMetadataOp) ApplyWorldOp(
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
		return false, errors.New("vm-image object not found")
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return true, err
	}
	if typeID != VmImageTypeID {
		return false, errors.Errorf("object %q is not a VmImage (type=%q)", objKey, typeID)
	}

	incoming := o.GetImage().CloneVT()
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		current, unmarshalErr := block.UnmarshalBlock[*VmImage](ctx, bcs, func() block.Block {
			return &VmImage{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if current == nil {
			return errors.New("vm-image block missing on object")
		}
		incoming.CreatedAt = current.GetCreatedAt()
		bcs.SetBlock(incoming, true)
		return nil
	})
	if err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *SetVmImageMetadataOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *SetVmImageMetadataOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *SetVmImageMetadataOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSetVmImageMetadataOp looks up a SetVmImageMetadataOp operation type.
func LookupSetVmImageMetadataOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == SetVmImageMetadataOpId {
		return &SetVmImageMetadataOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ block.Block = ((*VmImage)(nil))

// _ is a type assertion
var _ world.Operation = ((*CreateVmImageOp)(nil))

// _ is a type assertion
var _ world.Operation = ((*SetVmImageMetadataOp)(nil))
