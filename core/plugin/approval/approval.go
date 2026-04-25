package plugin_approval

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// PluginApprovalKeyPrefix is the prefix for plugin approval keys.
const PluginApprovalKeyPrefix = "plugin-approval"

// PluginApprovalKey returns the KV key for a plugin approval.
// Key format: plugin-approval/{space_id}/{manifest_id}
func PluginApprovalKey(spaceID, manifestID string) []byte {
	return []byte(strings.Join([]string{
		PluginApprovalKeyPrefix,
		spaceID,
		manifestID,
	}, "/"))
}

// GetPluginApproval reads the PluginApproval from the store.
// Returns nil, nil if the key is not found.
func GetPluginApproval(ctx context.Context, store kvtx.Store, spaceID, manifestID string) (*PluginApproval, error) {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	key := PluginApprovalKey(spaceID, manifestID)
	data, found, err := tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	approval := &PluginApproval{}
	if err := approval.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return approval, nil
}

// SetPluginApproval writes the PluginApproval to the store.
func SetPluginApproval(ctx context.Context, store kvtx.Store, spaceID, manifestID string, approval *PluginApproval) error {
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	data, err := approval.MarshalVT()
	if err != nil {
		return err
	}

	key := PluginApprovalKey(spaceID, manifestID)
	if err := tx.Set(ctx, key, data); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// IsApproved checks if the approval state is APPROVED.
func IsApproved(approval *PluginApproval) bool {
	return approval != nil && approval.GetState() == PluginApprovalState_PluginApprovalState_APPROVED
}

// DefaultVolumeID is the default volume ID for plugin approval lookups.
const DefaultVolumeID = ""

// DefaultObjectStoreID is the default object store ID for plugin approval lookups.
const DefaultObjectStoreID = "platform-account"

// CheckApproval checks if a manifest ID is approved for a space.
//
// volumeID defaults to PluginVolumeID if empty.
// objectStoreID defaults to "platform-account" if empty.
func CheckApproval(ctx context.Context, b bus.Bus, volumeID, objectStoreID, spaceID, manifestID string) (bool, error) {
	if volumeID == "" {
		volumeID = bldr_plugin.PluginVolumeID
	}
	if objectStoreID == "" {
		objectStoreID = DefaultObjectStoreID
	}
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		b,
		true,
		objectStoreID,
		volumeID,
		nil,
	)
	if err != nil {
		return false, err
	}
	defer ref.Release()

	store := handle.GetObjectStore()
	approval, err := GetPluginApproval(ctx, store, spaceID, manifestID)
	if err != nil {
		return false, err
	}
	return IsApproved(approval), nil
}

// GetApprovalState returns the approval state for a manifest ID in a space.
//
// volumeID defaults to PluginVolumeID if empty.
// objectStoreID defaults to "platform-account" if empty.
func GetApprovalState(ctx context.Context, b bus.Bus, volumeID, objectStoreID, spaceID, manifestID string) (PluginApprovalState, error) {
	if volumeID == "" {
		volumeID = bldr_plugin.PluginVolumeID
	}
	if objectStoreID == "" {
		objectStoreID = DefaultObjectStoreID
	}
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		b,
		true,
		objectStoreID,
		volumeID,
		nil,
	)
	if err != nil {
		return PluginApprovalState_PluginApprovalState_UNSPECIFIED, err
	}
	defer ref.Release()

	store := handle.GetObjectStore()
	approval, err := GetPluginApproval(ctx, store, spaceID, manifestID)
	if err != nil {
		return PluginApprovalState_PluginApprovalState_UNSPECIFIED, err
	}
	if approval == nil {
		return PluginApprovalState_PluginApprovalState_UNSPECIFIED, nil
	}
	return approval.GetState(), nil
}
