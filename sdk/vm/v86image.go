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

// V86ImageTypeID is the type identifier for V86Image objects.
const V86ImageTypeID = "spacewave/vm/image/v86"

// PredV86ImageWasm is the graph predicate for the emulator WASM binary.
var PredV86ImageWasm = quad.IRI("v86image/wasm")

// PredV86ImageBiosSeabios is the graph predicate for the SeaBIOS asset.
var PredV86ImageBiosSeabios = quad.IRI("v86image/bios/seabios")

// PredV86ImageBiosVgabios is the graph predicate for the VGA BIOS asset.
var PredV86ImageBiosVgabios = quad.IRI("v86image/bios/vgabios")

// PredV86ImageKernel is the graph predicate for the kernel image asset.
var PredV86ImageKernel = quad.IRI("v86image/kernel")

// PredV86ImageRootfs is the graph predicate for the rootfs archive asset.
var PredV86ImageRootfs = quad.IRI("v86image/rootfs")

// GetBlockTypeId returns the block type identifier.
func (v *V86Image) GetBlockTypeId() string {
	return V86ImageTypeID
}

// MarshalBlock marshals the block to binary.
func (v *V86Image) MarshalBlock() ([]byte, error) {
	return v.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (v *V86Image) UnmarshalBlock(data []byte) error {
	return v.UnmarshalVT(data)
}

// Validate performs cursory checks on the V86Image.
func (v *V86Image) Validate() error {
	if v.GetName() == "" {
		return errors.New("name is required")
	}
	if v.GetPlatform() != "v86" {
		return errors.New("platform must be v86")
	}
	return nil
}

// CreateV86ImageOpId is the operation id for CreateV86ImageOp.
var CreateV86ImageOpId = "spacewave/vm/image/v86/create"

// NewCreateV86ImageOp constructs a new CreateV86ImageOp.
func NewCreateV86ImageOp(objKey string, img *V86Image, ts time.Time) *CreateV86ImageOp {
	return &CreateV86ImageOp{
		ObjectKey: objKey,
		Image:     img,
		Timestamp: timestamppb.New(ts),
	}
}

// NewCreateV86ImageOpBlock constructs a new CreateV86ImageOp block.
func NewCreateV86ImageOpBlock() block.Block {
	return &CreateV86ImageOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateV86ImageOp) GetOperationTypeId() string {
	return CreateV86ImageOpId
}

// Validate performs cursory checks on the op.
func (o *CreateV86ImageOp) Validate() error {
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
func (o *CreateV86ImageOp) ApplyWorldOp(
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

	if err := world_types.SetObjectType(ctx, ws, objKey, V86ImageTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateV86ImageOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateV86ImageOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CreateV86ImageOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateV86ImageOp looks up a CreateV86ImageOp operation type.
func LookupCreateV86ImageOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateV86ImageOpId {
		return &CreateV86ImageOp{}, nil
	}
	return nil, nil
}

// SetV86ImageMetadataOpId is the operation id for SetV86ImageMetadataOp.
var SetV86ImageMetadataOpId = "spacewave/vm/image/v86/set-metadata"

// NewSetV86ImageMetadataOp constructs a new SetV86ImageMetadataOp.
func NewSetV86ImageMetadataOp(objKey string, img *V86Image) *SetV86ImageMetadataOp {
	return &SetV86ImageMetadataOp{
		ObjectKey: objKey,
		Image:     img,
	}
}

// NewSetV86ImageMetadataOpBlock constructs a new SetV86ImageMetadataOp block.
func NewSetV86ImageMetadataOpBlock() block.Block {
	return &SetV86ImageMetadataOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SetV86ImageMetadataOp) GetOperationTypeId() string {
	return SetV86ImageMetadataOpId
}

// Validate performs cursory checks on the op.
func (o *SetV86ImageMetadataOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if o.GetImage() == nil {
		return errors.New("image is required")
	}
	return o.GetImage().Validate()
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetV86ImageMetadataOp) ApplyWorldOp(
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
		return false, errors.New("v86 image object not found")
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return true, err
	}
	if typeID != V86ImageTypeID {
		return false, errors.Errorf("object %q is not a V86Image (type=%q)", objKey, typeID)
	}

	incoming := o.GetImage().CloneVT()
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		current, unmarshalErr := block.UnmarshalBlock[*V86Image](ctx, bcs, func() block.Block {
			return &V86Image{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if current == nil {
			return errors.New("v86 image block missing on object")
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
func (o *SetV86ImageMetadataOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *SetV86ImageMetadataOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *SetV86ImageMetadataOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSetV86ImageMetadataOp looks up a SetV86ImageMetadataOp operation type.
func LookupSetV86ImageMetadataOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == SetV86ImageMetadataOpId {
		return &SetV86ImageMetadataOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ block.Block = ((*V86Image)(nil))

// _ is a type assertion
var _ world.Operation = ((*CreateV86ImageOp)(nil))

// _ is a type assertion
var _ world.Operation = ((*SetV86ImageMetadataOp)(nil))
