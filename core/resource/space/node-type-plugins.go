package resource_space

import (
	"context"
	"slices"

	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_space_runtime "github.com/s4wave/spacewave/core/plugin/space/runtime"
	"github.com/s4wave/spacewave/db/world"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// HoldNodeTypePlugins runs the Space's listed plugins whose manifests hint at
// any of the requested node type IDs, until the stream ends. A hint only
// starts a plugin: the running plugin's registrations decide which node types
// resolve, so a stale hint costs an unneeded plugin load. The Space plugin
// runtime is held only while some plugin matches.
func (r *SpaceResource) HoldNodeTypePlugins(
	req *s4wave_space.HoldNodeTypePluginsRequest,
	strm s4wave_space.SRPCSpaceResourceService_HoldNodeTypePluginsStream,
) error {
	// Release the runtime and its demand when the stream ends.
	ctx := strm.Context()
	var demand *plugin_space_runtime.PluginDemand
	var releaseRuntime func()
	defer func() {
		if demand != nil {
			demand.Release()
			releaseRuntime()
		}
	}()

	// Recompute the matching plugins after each change to the manifests read.
	engine := r.space.GetWorldEngine()
	var prev []string
	for first := true; ; first = false {
		// Read the hinted plugins from one World snapshot.
		reads := world.NewReadSet()
		pluginIDs, seqno, err := r.readNodeTypePlugins(ctx, engine, reads, req.GetNodeTypeIds())
		if err != nil {
			return err
		}

		// Demand the matching plugins, acquiring the runtime for the first one.
		if demand == nil && len(pluginIDs) != 0 {
			_, runtime, runtimeRef, err := r.AcquireContentsRuntime(ctx)
			if err != nil {
				return err
			}
			demand, releaseRuntime = runtime.DemandPlugins(nil), runtimeRef.Release
		}
		if demand != nil {
			demand.SetPluginIDs(pluginIDs)
		}

		// Report the held plugins when they change.
		if first || !slices.Equal(prev, pluginIDs) {
			if err := strm.Send(&s4wave_space.HoldNodeTypePluginsResponse{PluginIds: pluginIDs}); err != nil {
				return err
			}
			prev = pluginIDs
		}
		if _, err := reads.WaitChange(ctx, engine, seqno); err != nil {
			return err
		}
	}
}

// readNodeTypePlugins returns the sorted plugins whose manifests hint at any of
// typeIDs, and the World revision read, recording the reads in reads.
func (r *SpaceResource) readNodeTypePlugins(
	ctx context.Context,
	engine world.Engine,
	reads *world.ReadSet,
	typeIDs []string,
) ([]string, uint64, error) {
	// Open a snapshot and capture its revision for the next change.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Discard()
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Read the manifest catalog and select the plugins hinting at a type.
	wtx := reads.Track(tx)
	manifestRefs, err := collectAvailablePluginManifestRefs(ctx, wtx)
	if err != nil {
		return nil, 0, err
	}
	wanted := make(map[string]struct{}, len(typeIDs))
	for _, id := range typeIDs {
		wanted[id] = struct{}{}
	}
	plugins, err := collectAvailablePlugins(ctx, wtx, manifestRefs, bldr_manifest_world.LookupManifest)
	if err != nil {
		return nil, 0, err
	}
	var pluginIDs []string
	for _, plugin := range plugins {
		if slices.ContainsFunc(plugin.GetNodeTypeIds(), func(id string) bool { _, ok := wanted[id]; return ok }) {
			pluginIDs = append(pluginIDs, plugin.GetPluginId())
		}
	}
	return pluginIDs, seqno, nil
}
