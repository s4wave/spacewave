package bldr_manifest_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// StoreManifestOpId is the operation ID for StoreManifest.
var StoreManifestOpId = "bldr/manifest/store"

// ExStoreManifestOp stores a manifest to an object key.
func ExStoreManifestOp(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objectKey string,
	linkObjKeys []string,
	manifestRef *manifest.ManifestRef,
) error {
	op := NewStoreManifestOp(
		objectKey,
		linkObjKeys,
		manifestRef,
	)
	_, _, err := ws.ApplyWorldOp(op, sender)
	return err
}

// NewStoreManifestOp constructs a new StoreManifestOp block.
func NewStoreManifestOp(
	objectKey string,
	linkObjectKeys []string,
	manifestRef *manifest.ManifestRef,
) *StoreManifestOp {
	return &StoreManifestOp{
		ObjectKey:      objectKey,
		LinkObjectKeys: linkObjectKeys,
		ManifestRef:    manifestRef.CloneVT(),
	}
}

// NewStoreManifestOpBlock constructs a new StoreManifestOp block.
func NewStoreManifestOpBlock() block.Block {
	return &StoreManifestOp{}
}

// Validate performs cursory checks on the op.
func (o *StoreManifestOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetManifestRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *StoreManifestOp) GetOperationTypeId() string {
	return StoreManifestOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *StoreManifestOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// unmarshal the manifest
	var out *manifest.Manifest
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, o.GetManifestRef().GetManifestRef(), func(bcs *block.Cursor) error {
		var err error
		out, err = manifest.UnmarshalManifest(bcs)
		return err
	})
	if err != nil {
		return false, err
	}

	// store the object for the manifest
	_, err = SetManifest(ctx, ws, sender, o.GetObjectKey(), manifestRef)
	if err != nil {
		return false, err
	}

	// link any LinkObjectKeys
	for _, objKey := range o.GetLinkObjectKeys() {
		quad := NewManifestQuad(objKey, o.GetObjectKey(), out.GetMeta().GetManifestId())
		if err := ws.SetGraphQuad(quad); err != nil {
			return false, err
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *StoreManifestOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *StoreManifestOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *StoreManifestOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*StoreManifestOp)(nil))
