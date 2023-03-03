package plugin_host

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// StorePluginManifestOpId is the operation ID for StorePluginManifest.
var StorePluginManifestOpId = "bldr/plugin/host/plugin-manifest/store"

// ExStorePluginManifestOp stores a plugin manifest to an object key.
func ExStorePluginManifestOp(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objectKey string,
	linkObjKeys []string,
	pluginMeta *plugin.PluginManifestMeta,
	pluginManifestRef *bucket.ObjectRef,
) error {
	op := NewStorePluginManifestOp(
		objectKey,
		linkObjKeys,
		pluginMeta,
		pluginManifestRef,
	)
	_, _, err := ws.ApplyWorldOp(op, sender)
	return err
}

// NewStorePluginManifestOp constructs a new StorePluginManifestOp block.
func NewStorePluginManifestOp(
	objectKey string,
	linkObjectKeys []string,
	pluginManifestMeta *plugin.PluginManifestMeta,
	pluginManifestRef *bucket.ObjectRef,
) *StorePluginManifestOp {
	return &StorePluginManifestOp{
		ObjectKey:          objectKey,
		LinkObjectKeys:     linkObjectKeys,
		PluginManifestMeta: pluginManifestMeta.CloneVT(),
		PluginManifest:     pluginManifestRef.Clone(),
	}
}

// NewStorePluginManifestOpBlock constructs a new StorePluginManifestOp block.
func NewStorePluginManifestOpBlock() block.Block {
	return &StorePluginManifestOp{}
}

// Validate performs cursory checks on the op.
func (o *StorePluginManifestOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetPluginManifest().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *StorePluginManifestOp) GetOperationTypeId() string {
	return StorePluginManifestOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *StorePluginManifestOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// unmarshal the manifest
	var manifest *plugin.PluginManifest
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, o.GetPluginManifest(), func(bcs *block.Cursor) error {
		var err error
		manifest, err = plugin.UnmarshalPluginManifest(bcs)
		return err
	})
	if err != nil {
		return false, err
	}

	// store the object for the plugin manifest
	_, err = SetPluginManifest(ctx, ws, sender, o.GetObjectKey(), manifestRef)
	if err != nil {
		return false, err
	}

	// link any LinkObjectKeys
	for _, objKey := range o.GetLinkObjectKeys() {
		quad := NewPluginQuad(objKey, o.GetObjectKey(), manifest.GetMeta().GetPluginId())
		if err := ws.SetGraphQuad(quad); err != nil {
			return false, err
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *StorePluginManifestOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *StorePluginManifestOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *StorePluginManifestOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*StorePluginManifestOp)(nil))
