package plugin_space

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/directive"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// resolverEntry tracks an active FetchManifest resolver.
type resolverEntry struct {
	// ctx is the resolver context.
	ctx context.Context
	// dir is the FetchManifest directive.
	dir manifest.FetchManifest
	// handler is the resolver handler for emitting values.
	handler directive.ResolverHandler
	// emitted is the previously emitted value for diffing.
	emitted *manifest.FetchManifestValue
	// seen is the inputs the entry last resolved against, or nil before its
	// first resolve.
	seen *resolverInputs
}

// resolverInputs is what resolving one entry reads: whether SpaceSettings lists
// its manifest ID, and the manifests reachable from the search roots with the
// roots of their objects.
type resolverInputs struct {
	// listed reports that SpaceSettings lists the manifest ID.
	listed bool
	// manifests is the reachable manifest objects, sorted by key. It is empty
	// when the manifest ID is not listed.
	manifests []*world.ObjectRootRef
}

// equal reports whether resolving against i gives the result of resolving
// against other.
func (i *resolverInputs) equal(other *resolverInputs) bool {
	return i.listed == other.listed && slices.EqualFunc(i.manifests, other.manifests, func(a, b *world.ObjectRootRef) bool {
		return a.ObjectKey == b.ObjectKey && a.Exists == b.Exists && a.RootRef.EqualVT(b.RootRef)
	})
}

// processResolvers processes all active FetchManifest resolvers against the
// current world state. Every World change reaches it, but an entry resolves
// again only when its inputs changed: its manifest ID entering or leaving the
// SpaceSettings plugin list, or a change to the manifests reachable from the
// manifest stores. It reads the inputs from the same World state it resolves
// from, so a change after the read starts another pass.
func (c *Controller) processResolvers(ctx context.Context, ws world.WorldState) {
	// Trace the resolver processing task.
	ctx, task := trace.NewTask(ctx, "core/plugin-space/fetch-manifest/process-resolvers")
	defer task.End()

	// Snapshot the resolver set and current plugin IDs.
	var entries []*resolverEntry
	var ids []string
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entries = make([]*resolverEntry, 0, len(c.resolvers))
		for e := range c.resolvers {
			entries = append(entries, e)
		}
		ids = c.pluginIDs
	})

	// Trace the resolver and plugin counts.
	trace.Logf(ctx, "resolver-count", "%d", len(entries))
	trace.Log(ctx, "plugin-ids", strings.Join(ids, ","))

	// Return when no resolvers are active.
	if len(entries) == 0 {
		return
	}

	// Read the manifests once for the entries whose manifest ID is listed.
	le := c.GetLogger()
	var objKeys []string
	var manifestRoots []*world.ObjectRootRef
	var rootsErr error
	if slices.ContainsFunc(entries, func(e *resolverEntry) bool {
		return slices.Contains(ids, e.dir.GetManifestId())
	}) {
		objKeys, manifestRoots, rootsErr = c.readManifestRoots(ctx, ws)
		if rootsErr != nil {
			warnOnErrorUnlessCanceled(ctx, le, rootsErr, "failed to list manifests")
			trace.Log(ctx, "result", "list-manifests-error")
		}
	}

	// Resolve each entry whose inputs changed.
	for _, entry := range entries {
		if entry.ctx.Err() != nil {
			continue
		}

		// Skip an entry that already resolved against these inputs. An entry
		// that could not read its inputs resolves on the next World change.
		mid := entry.dir.GetManifestId()
		inputs := &resolverInputs{listed: slices.Contains(ids, mid)}
		if inputs.listed {
			if rootsErr != nil {
				continue
			}
			inputs.manifests = manifestRoots
		}
		if entry.seen != nil && entry.seen.equal(inputs) {
			continue
		}

		// Trace one resolver entry.
		entryCtx, entryTask := trace.NewTask(ctx, "core/plugin-space/fetch-manifest/resolve")
		trace.Log(entryCtx, "manifest-id", mid)
		trace.Log(entryCtx, "platform-ids", strings.Join(entry.dir.GetPlatformIds(), ","))

		// Skip manifests not in the current plugin list.
		if !inputs.listed {
			le.WithField("manifest-id", mid).Debug("manifest not in plugin list, skipping")
			trace.Log(entryCtx, "result", "skipped-plugin-id")
			if entry.emitted != nil {
				_ = entry.handler.ClearValues()
				entry.emitted = nil
			}
			entry.handler.MarkIdle(true)
			entry.seen = inputs
			entryTask.End()
			continue
		}

		le.WithField("manifest-id", mid).WithField("obj-keys", objKeys).Debug("searching for manifests")
		trace.Log(entryCtx, "object-keys", strings.Join(objKeys, ","))

		// Collect manifests from the Space world.
		manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
			entryCtx, ws, mid, entry.dir.GetPlatformIds(), objKeys...,
		)
		if err != nil {
			warnOnErrorUnlessCanceled(entryCtx, le.WithField("manifest-id", mid), err, "failed to collect manifests")
			trace.Log(entryCtx, "result", "collect-error")
			entryTask.End()
			continue
		}
		for _, merr := range manifestErrs {
			warnOnErrorUnlessCanceled(entryCtx, le, merr, "ignoring invalid manifest")
		}
		le.WithField("manifest-id", mid).WithField("count", len(manifests)).Debug("collected manifests")
		trace.Logf(entryCtx, "collected-count", "%d", len(manifests))
		trace.Logf(entryCtx, "invalid-count", "%d", len(manifestErrs))

		// Filter by build types, min revision, and latest revision.
		manifests = bldr_manifest_world.FilterCollectedManifestsByBuildTypes(manifests, entry.dir.GetBuildTypes())
		manifests = bldr_manifest_world.FilterCollectedManifestsByMinRev(manifests, entry.dir.GetRev())
		manifests = bldr_manifest_world.FilterCollectedManifestsByLatestRev(manifests)
		trace.Logf(entryCtx, "emitted-count", "%d", len(manifests))

		// Build ManifestRef list.
		refs := make([]*manifest.ManifestRef, len(manifests))
		for i, m := range manifests {
			refs[i] = &manifest.ManifestRef{
				Meta:        m.Manifest.Meta,
				ManifestRef: m.ManifestRef,
			}
		}

		// Diff against previous value. An empty result adds no value, so it
		// never masks manifests the parent source supplies.
		next := &manifest.FetchManifestValue{ManifestRefs: refs}
		if entry.emitted == nil || !next.EqualVT(entry.emitted) {
			_ = entry.handler.ClearValues()
			if len(refs) != 0 {
				_, _ = entry.handler.AddValue(next)
			}
			entry.emitted = next
			le.WithField("manifest-id", mid).Debugf("resolved %d manifest(s)", len(manifests))
		}
		entry.handler.MarkIdle(true)
		entry.seen = inputs
		trace.Log(entryCtx, "result", "resolved")
		entryTask.End()
	}
}

// readManifestRoots lists the manifest search roots and the manifests reachable
// from them with their object roots. It searches from the configured objects,
// or from every manifest store. Collection follows <manifest> edges out of
// these objects, and deploys link each Manifest from its store, so the stores
// are the roots.
func (c *Controller) readManifestRoots(ctx context.Context, ws world.WorldState) ([]string, []*world.ObjectRootRef, error) {
	objKeys := c.GetConfig().GetObjectKeys()
	if len(objKeys) == 0 {
		var err error
		objKeys, err = world_types.ListObjectsWithType(ctx, ws, bldr_manifest_world.ManifestStoreTypeID)
		if err != nil {
			return nil, nil, err
		}
	}
	manifestKeys, err := bldr_manifest_world.ListManifests(ctx, ws, objKeys...)
	if err != nil {
		return nil, nil, err
	}
	roots, err := world.GetObjectRootRefsBatch(ctx, ws, manifestKeys)
	if err != nil {
		return nil, nil, err
	}
	return objKeys, roots, nil
}
