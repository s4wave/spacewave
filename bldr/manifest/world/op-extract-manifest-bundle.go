package bldr_manifest_world

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ExtractManifestBundleOpId is the operation ID for ExtractManifestBundle.
var ExtractManifestBundleOpId = "bldr/manifest/bundle/extract"

// ExExtractManifestBundleOp extracts a manifest bundle with an operation.
func ExExtractManifestBundleOp(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objectKey string,
	linkObjKeys []string,
	manifestBundleRef *bucket.ObjectRef,
) error {
	op := NewExtractManifestBundleOp(
		objectKey,
		linkObjKeys,
		manifestBundleRef,
	)
	_, _, err := ws.ApplyWorldOp(ctx, op, sender)
	return err
}

// NewExtractManifestBundleOp constructs a new ExtractManifestBundleOp block.
func NewExtractManifestBundleOp(
	objectKey string,
	linkObjectKeys []string,
	manifestBundleRef *bucket.ObjectRef,
) *ExtractManifestBundleOp {
	return &ExtractManifestBundleOp{
		ObjectKey:      objectKey,
		LinkObjectKeys: linkObjectKeys,
		ManifestBundle: manifestBundleRef,
	}
}

// NewExtractManifestBundleOpBlock constructs a new ExtractManifestBundleOp block.
func NewExtractManifestBundleOpBlock() block.Block {
	return &ExtractManifestBundleOp{}
}

// Validate performs cursory checks on the op.
func (o *ExtractManifestBundleOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetManifestBundle().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *ExtractManifestBundleOp) GetOperationTypeId() string {
	return ExtractManifestBundleOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ExtractManifestBundleOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// store the object for the manifest
	_, _, _, err = ExtractManifestBundle(ctx, ws, sender, o.GetObjectKey(), o.GetManifestBundle())
	if err != nil {
		return false, err
	}

	for _, objKey := range o.GetLinkObjectKeys() {
		quad := NewManifestQuad(objKey, o.GetObjectKey(), "")
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
			return false, err
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ExtractManifestBundleOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *ExtractManifestBundleOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *ExtractManifestBundleOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ExtractManifestBundleOp)(nil))
