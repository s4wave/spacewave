package plugin_host

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// UpdatePluginManifestOpId is the unixfs operation id.
var UpdatePluginManifestOpId = "bldr/plugin/host/update-plugin-manifest"

// UpdatePluginManifest updates a PluginManifest attached to a PluginHost.
func UpdatePluginManifest(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	pluginHostObjKey string,
	pluginID string,
	pluginManifestRef *bucket.ObjectRef,
) error {
	op := NewUpdatePluginManifestOp(
		pluginHostObjKey,
		pluginID,
		pluginManifestRef,
	)
	_, _, err := ws.ApplyWorldOp(op, sender)
	return err
}

// NewUpdatePluginManifestOp constructs a new UpdatePluginManifestOp block.
// repoRef, worktreeArgs can be empty
func NewUpdatePluginManifestOp(
	pluginHostObjKey string,
	pluginID string,
	pluginManifestRef *bucket.ObjectRef,
) *UpdatePluginManifestOp {
	return &UpdatePluginManifestOp{
		PluginHostKey:  pluginHostObjKey,
		PluginId:       pluginID,
		PluginManifest: pluginManifestRef.Clone(),
	}
}

// NewUpdatePluginManifestOpBlock constructs a new UpdatePluginManifestOp block.
func NewUpdatePluginManifestOpBlock() block.Block {
	return &UpdatePluginManifestOp{}
}

// Validate performs cursory checks on the op.
func (o *UpdatePluginManifestOp) Validate() error {
	if len(o.GetPluginHostKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := plugin.ValidatePluginID(o.GetPluginId()); err != nil {
		return err
	}
	if err := o.GetPluginManifest().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *UpdatePluginManifestOp) GetOperationTypeId() string {
	return UpdatePluginManifestOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *UpdatePluginManifestOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	typesState := world_types.NewTypesState(ctx, ws)
	pluginHostKey := o.GetPluginHostKey()

	// check pluginHostKey exists
	_, err = world.MustGetObject(ws, pluginHostKey)
	if err != nil {
		return false, errors.Wrap(err, "plugin host")
	}

	// check if pluginHostKey is indeed a PluginHost
	err = CheckPluginHostType(typesState, pluginHostKey)
	if err != nil {
		return false, err
	}

	// delete any existing manifest linked with this id
	pluginID := o.GetPluginId()
	pluginManifestKey := NewPluginHostPluginManifestKey(pluginHostKey, pluginID)
	_, existingKey, err := LookupPluginHostManifest(ctx, ws, pluginHostKey, pluginID)
	if err != nil {
		return false, err
	}
	if existingKey != pluginManifestKey {
		_, err = ws.DeleteObject(existingKey)
		if err != nil {
			return false, err
		}
	}

	// store the object for the plugin manifest
	_, err = SetPluginManifest(ctx, ws, sender, pluginManifestKey, o.GetPluginManifest())
	if err != nil {
		return false, err
	}

	// link the manifest with the plugin host
	err = ws.SetGraphQuad(NewPluginHostToPluginManifestQuad(pluginHostKey, pluginManifestKey, pluginID))
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *UpdatePluginManifestOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *UpdatePluginManifestOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *UpdatePluginManifestOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*UpdatePluginManifestOp)(nil))
