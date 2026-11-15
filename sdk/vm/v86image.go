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

// NewCreateV86ImageOp constructs a new CreateV86ImageOp.
func NewCreateV86ImageOp(objKey string, img *V86Image, ts time.Time) *CreateV86ImageOp {
	return &CreateV86ImageOp{
		ObjectKey: objKey,
		Image:     img,
		Timestamp: timestamppb.New(ts),
	}
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
	// Validate the VM image creation intent.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Prepare a retained image with the operation's creation timestamp.
	objKey := o.GetObjectKey()
	img := o.GetImage().CloneVT()
	img.CreatedAt = o.GetTimestamp()

	// Create the VM image object with its initial metadata block.
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(img, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Mark the VM image object with its block type.
	if err := world_types.SetObjectType(ctx, ws, objKey, V86ImageTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// NewSetV86ImageMetadataOp constructs a new SetV86ImageMetadataOp.
func NewSetV86ImageMetadataOp(objKey string, img *V86Image) *SetV86ImageMetadataOp {
	return &SetV86ImageMetadataOp{
		ObjectKey: objKey,
		Image:     img,
	}
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
	// Validate the VM image metadata update.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Open the VM image object for its metadata change.
	objKey := o.GetObjectKey()
	objState, found, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return true, err
	}
	if !found {
		return false, world.NewOpRefusedError(errors.New("v86 image object not found"))
	}

	// Require the opened object to carry the VM image block type.
	typeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return true, err
	}
	if typeID != V86ImageTypeID {
		return false, world.NewOpRefusedError(errors.Errorf("object %q is not a V86Image (type=%q)", objKey, typeID))
	}

	// Replace the VM image metadata while retaining its creation timestamp.
	incoming := o.GetImage().CloneVT()
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		// Decode the current VM image and require its retained metadata.
		current, unmarshalErr := block.UnmarshalBlock[V86Image](ctx, bcs)
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if current == nil {
			return world.NewOpRefusedError(errors.New("v86 image block missing on object"))
		}

		// Preserve the image's creation timestamp in the replacement block.
		incoming.CreatedAt = current.GetCreatedAt()
		bcs.SetBlock(incoming, true)
		return nil
	})
	if err != nil {
		return !world.IsOpRefusedError(err), err
	}

	return false, nil
}

var _ block.Block = (*V86Image)(nil)

var _ world.Operation = (*CreateV86ImageOp)(nil)

var _ world.Operation = (*SetV86ImageMetadataOp)(nil)
