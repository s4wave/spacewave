package space_world_ops

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/labels"
	"github.com/sirupsen/logrus"
)

// Validate checks the timestamp and each plugin manifest ID.
func (o *AddSpacePluginsOp) Validate() error {
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	for _, pluginID := range o.GetPluginIds() {
		if err := labels.ValidateDNSLabel(pluginID); err != nil {
			return errors.Wrap(err, "plugin_ids")
		}
	}
	return nil
}

// ApplyWorldOp adds the plugin IDs to the transaction's current settings.
//
// The union is taken under the World writer, so an ID another writer added
// before this op is kept, and every other setting, such as the index path, is
// unchanged. Missing settings are created. Settings that already list every
// ID are not rewritten.
func (o *AddSpacePluginsOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	// Read the transaction's current settings, which may not exist yet.
	current, err := space_world.LookupSpaceSettingsBody(ctx, ws)
	if err != nil {
		return false, err
	}
	settings := &space_world.SpaceSettings{}
	if current != nil {
		settings = current.CloneVT()
	}

	// Leave settings that already list every ID untouched.
	if !addSpacePluginIDs(settings, o.GetPluginIds()...) && current != nil {
		return false, nil
	}

	// Write the union through a SetSpaceSettings operation.
	update := &SetSpaceSettingsOp{
		Settings:  settings,
		Overwrite: true,
		Timestamp: o.GetTimestamp(),
	}
	return update.ApplyWorldOp(ctx, le, ws, sender)
}

// addSpacePluginIDs appends each plugin ID settings does not list yet, in
// order, and reports whether the list changed. It also drops listed IDs that
// are not valid manifest IDs, so a corrupted settings block is not written
// back.
func addSpacePluginIDs(settings *space_world.SpaceSettings, pluginIDs ...string) bool {
	// Drop listed IDs that are not valid, remembering the ones kept.
	listed := len(settings.PluginIds)
	settings.PluginIds = slices.DeleteFunc(settings.PluginIds, func(pluginID string) bool {
		return labels.ValidateDNSLabel(pluginID) != nil
	})
	changed := len(settings.PluginIds) != listed
	present := make(map[string]struct{}, len(settings.PluginIds)+len(pluginIDs))
	for _, pluginID := range settings.PluginIds {
		present[pluginID] = struct{}{}
	}

	// Append each requested ID not listed yet, in order.
	for _, pluginID := range pluginIDs {
		if _, ok := present[pluginID]; ok {
			continue
		}
		present[pluginID] = struct{}{}
		settings.PluginIds = append(settings.PluginIds, pluginID)
		changed = true
	}
	return changed
}

var _ world.Operation = (*AddSpacePluginsOp)(nil)
