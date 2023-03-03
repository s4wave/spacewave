package plugin_host

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// ExtractPluginManifestBundleOpId is the operation ID for ExtractPluginManifestBundle.
var ExtractPluginManifestBundleOpId = "bldr/plugin/host/plugin-manifest-bundle/extract"

// ExExtractPluginManifestBundleOp extracts a plugin manifest bundle with an operation.
func ExExtractPluginManifestBundleOp(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objectKey string,
	linkObjKeys []string,
	pluginManifestBundleRef *bucket.ObjectRef,
) error {
	op := NewExtractPluginManifestBundleOp(
		objectKey,
		linkObjKeys,
		pluginManifestBundleRef,
	)
	_, _, err := ws.ApplyWorldOp(op, sender)
	return err
}

// NewExtractPluginManifestBundleOp constructs a new ExtractPluginManifestBundleOp block.
func NewExtractPluginManifestBundleOp(
	objectKey string,
	linkObjectKeys []string,
	pluginManifestBundleRef *bucket.ObjectRef,
) *ExtractPluginManifestBundleOp {
	return &ExtractPluginManifestBundleOp{
		ObjectKey:            objectKey,
		LinkObjectKeys:       linkObjectKeys,
		PluginManifestBundle: pluginManifestBundleRef,
	}
}

// NewExtractPluginManifestBundleOpBlock constructs a new ExtractPluginManifestBundleOp block.
func NewExtractPluginManifestBundleOpBlock() block.Block {
	return &ExtractPluginManifestBundleOp{}
}

// Validate performs cursory checks on the op.
func (o *ExtractPluginManifestBundleOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetPluginManifestBundle().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *ExtractPluginManifestBundleOp) GetOperationTypeId() string {
	return ExtractPluginManifestBundleOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ExtractPluginManifestBundleOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// store the object for the plugin manifest
	_, _, _, err = ExtractPluginManifestBundle(ctx, ws, sender, o.GetObjectKey(), o.GetPluginManifestBundle())
	if err != nil {
		return false, err
	}

	for _, objKey := range o.GetLinkObjectKeys() {
		quad := NewPluginQuad(objKey, o.GetObjectKey(), "")
		if err := ws.SetGraphQuad(quad); err != nil {
			return false, err
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ExtractPluginManifestBundleOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *ExtractPluginManifestBundleOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *ExtractPluginManifestBundleOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ExtractPluginManifestBundleOp)(nil))
