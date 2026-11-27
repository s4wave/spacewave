package resource_space

import (
	"context"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// pluginHostObjectKey is the manifest store a deploy publishes to by default.
const pluginHostObjectKey = "plugin-host"

// ImportSpacePlugin copies a plugin's newest build for each platform from
// another mounted Space into this one.
func (r *SpaceResource) ImportSpacePlugin(
	ctx context.Context,
	req *s4wave_space.ImportSpacePluginRequest,
) (*s4wave_space.ImportSpacePluginResponse, error) {
	// Resolve the source Space from the calling resource client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	srcValue, err := resourceCtx.GetResourceValue(req.GetSourceSpaceResourceId())
	if err != nil {
		return nil, err
	}
	src, ok := srcValue.(*SpaceResource)
	if !ok || src == nil {
		return nil, errors.New("source_space_resource_id is not a Space")
	}
	if err := r.importPlugin(ctx, src, req.GetPluginId()); err != nil {
		return nil, err
	}
	return &s4wave_space.ImportSpacePluginResponse{}, nil
}

// importPlugin publishes the newest build of pluginID for each platform in
// src into this Space's plugin host, reading only the blocks this Space lacks.
func (r *SpaceResource) importPlugin(ctx context.Context, src *SpaceResource, pluginID string) error {
	// Require a plugin from a different Space.
	if pluginID == "" {
		return errors.New("plugin_id is required")
	}
	if src.space.GetWorldEngineID() == r.space.GetWorldEngineID() {
		return errors.New("source Space is this Space")
	}

	// Read the source catalog and blocks from one World snapshot.
	tx, err := src.space.GetWorldEngine().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Select the newest build of the plugin for each platform.
	refs, err := collectNewestPluginBuilds(ctx, tx, pluginID)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return errors.Errorf("source Space has no build of %s", pluginID)
	}

	// Copy the builds from the source block store and publish them.
	cursor, err := tx.BuildStorageCursor(ctx)
	if err != nil {
		return errors.Wrap(err, "build source storage cursor")
	}
	defer cursor.Release()
	return r.publishManifestSet(ctx, pluginHostObjectKey, refs, cursor.GetBlockStore())
}

// collectNewestPluginBuilds returns the highest revision readable manifest of
// pluginID for each platform in ws.
func collectNewestPluginBuilds(ctx context.Context, ws world.WorldState, pluginID string) ([]*bldr_manifest.ManifestRef, error) {
	// List every manifest object in the World.
	keys, err := world_types.ListObjectsWithType(ctx, ws, bldr_manifest_world.ManifestTypeID)
	if err != nil {
		return nil, err
	}

	// Keep the highest revision of pluginID for each platform.
	var refs []*bldr_manifest.ManifestRef
	platformIdx := make(map[string]int)
	for _, key := range keys {
		// Skip a manifest the Space cannot read, as the plugin catalog does.
		manifest, objRef, err := bldr_manifest_world.LookupManifest(ctx, ws, key)
		if err != nil || manifest == nil {
			continue
		}
		meta := manifest.GetMeta()
		if meta.GetManifestId() != pluginID {
			continue
		}
		ref := &bldr_manifest.ManifestRef{Meta: meta, ManifestRef: objRef}
		idx, ok := platformIdx[meta.GetPlatformId()]
		if !ok {
			platformIdx[meta.GetPlatformId()] = len(refs)
			refs = append(refs, ref)
		} else if meta.GetRev() > refs[idx].GetMeta().GetRev() {
			refs[idx] = ref
		}
	}
	return refs, nil
}
