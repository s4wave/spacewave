package plugin_list

import (
	"context"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
)

// checkPluginLoaded checks if a plugin is currently loaded and running.
// Uses ExLoadPlugin with returnIfIdle=true for a non-blocking check.
func checkPluginLoaded(ctx context.Context, b bus.Bus, pluginID string) bool {
	rp, _, ref, err := bldr_plugin.ExLoadPlugin(ctx, b, true, pluginID, nil)
	if ref != nil {
		ref.Release()
	}
	return err == nil && rp != nil
}

// fetchManifestInfo fetches manifest metadata for a plugin via FetchManifest.
// Returns nil if no manifest is found or on error.
func fetchManifestInfo(ctx context.Context, b bus.Bus, pluginID string) *ManifestInfo {
	dir := bldr_manifest.NewFetchManifest(pluginID, nil, nil, 0)
	val, _, ref, err := bus.ExecOneOffTyped[*bldr_manifest.FetchManifestValue](
		ctx, b, dir, bus.ReturnWhenIdle(), nil,
	)
	if ref != nil {
		ref.Release()
	}
	if err != nil || val == nil {
		return nil
	}

	fmv := val.GetValue()
	refs := fmv.GetManifestRefs()
	if len(refs) == 0 {
		return nil
	}

	// Use the first manifest ref for name/description/version.
	first := refs[0].GetMeta()
	info := &ManifestInfo{
		Name:        first.GetManifestId(),
		Description: first.GetDescription(),
		Version:     strconv.FormatUint(first.GetRev(), 10),
	}

	// Collect unique build types across all manifest refs.
	seen := make(map[string]struct{})
	for _, r := range refs {
		bt := r.GetMeta().GetBuildType()
		if bt == "" {
			continue
		}
		if _, ok := seen[bt]; ok {
			continue
		}
		seen[bt] = struct{}{}
		info.BuildTypes = append(info.BuildTypes, bt)
	}

	return info
}

// listAvailablePluginsResolver resolves the ListAvailablePlugins directive.
type listAvailablePluginsResolver struct {
	// b is the bus.
	b bus.Bus
	// dir is the directive.
	dir ListAvailablePlugins
	// pluginIDs is the set of plugin manifest IDs declared in the Space.
	pluginIDs []string
	// volumeID is the volume ID for the KV store.
	volumeID string
	// objectStoreID is the object store ID for approval lookups.
	objectStoreID string
}

// NewResolver constructs a resolver for ListAvailablePlugins.
//
// pluginIDs is the set of manifest IDs declared in the Space.
// volumeID and objectStoreID configure KV store access for approval lookups.
// If empty, the defaults from plugin_approval.CheckApproval are used.
func NewResolver(
	b bus.Bus,
	dir ListAvailablePlugins,
	pluginIDs []string,
	volumeID string,
	objectStoreID string,
) directive.Resolver {
	return &listAvailablePluginsResolver{
		b:             b,
		dir:           dir,
		pluginIDs:     pluginIDs,
		volumeID:      volumeID,
		objectStoreID: objectStoreID,
	}
}

// Resolve resolves the values, emitting them to the handler.
func (r *listAvailablePluginsResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	spaceID := r.dir.ListAvailablePluginsSpaceID()
	if len(r.pluginIDs) == 0 {
		handler.AddValue(&AvailablePluginList{})
		return nil
	}

	plugins := make([]*AvailablePlugin, 0, len(r.pluginIDs))
	for _, pid := range r.pluginIDs {
		state, err := plugin_approval.GetApprovalState(
			ctx,
			r.b,
			r.volumeID,
			r.objectStoreID,
			spaceID,
			pid,
		)
		if err != nil {
			return err
		}

		loaded := checkPluginLoaded(ctx, r.b, pid)
		info := fetchManifestInfo(ctx, r.b, pid)
		plugins = append(plugins, &AvailablePlugin{
			ManifestID:   pid,
			Approved:     state,
			Loaded:       loaded,
			ManifestInfo: info,
		})
	}

	handler.AddValue(&AvailablePluginList{Plugins: plugins})
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*listAvailablePluginsResolver)(nil))
