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
}

// processResolvers processes all active FetchManifest resolvers against the current world state.
func (c *Controller) processResolvers(ctx context.Context, ws world.WorldState) {
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

	le := c.GetLogger()
	le.WithField("entries", len(entries)).WithField("plugin-ids", ids).Debug("processResolvers called")
	trace.Logf(ctx, "resolver-count", "%d", len(entries))
	trace.Log(ctx, "plugin-ids", strings.Join(ids, ","))

	if len(entries) == 0 {
		return
	}

	conf := c.GetConfig()

	for _, entry := range entries {
		entryCtx, entryTask := trace.NewTask(ctx, "core/plugin-space/fetch-manifest/resolve")
		if entry.ctx.Err() != nil {
			entryTask.End()
			continue
		}

		mid := entry.dir.GetManifestId()
		trace.Log(entryCtx, "manifest-id", mid)
		trace.Log(entryCtx, "platform-ids", strings.Join(entry.dir.GetPlatformIds(), ","))

		// Skip manifests not in the current plugin list.
		if !slices.Contains(ids, mid) {
			le.WithField("manifest-id", mid).Debug("manifest not in plugin list, skipping")
			trace.Log(entryCtx, "result", "skipped-plugin-id")
			if entry.emitted != nil {
				_ = entry.handler.ClearValues()
				entry.emitted = nil
			}
			entry.handler.MarkIdle(true)
			entryTask.End()
			continue
		}

		// Determine object keys to search for manifests.
		objKeys := conf.GetObjectKeys()
		if len(objKeys) == 0 {
			var listErr error
			objKeys, listErr = world_types.ListObjectsWithType(entryCtx, ws, bldr_manifest_world.ManifestTypeID)
			if listErr != nil {
				le.WithError(listErr).Warn("failed to list manifest objects")
				trace.Log(entryCtx, "result", "list-objects-error")
				entryTask.End()
				continue
			}
		}
		le.WithField("manifest-id", mid).WithField("obj-keys", objKeys).Debug("searching for manifests")
		trace.Log(entryCtx, "object-keys", strings.Join(objKeys, ","))

		// Collect manifests from the Space world.
		manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
			entryCtx, ws, mid, entry.dir.GetPlatformIds(), objKeys...,
		)
		if err != nil {
			le.WithError(err).WithField("manifest-id", mid).Warn("failed to collect manifests")
			trace.Log(entryCtx, "result", "collect-error")
			entryTask.End()
			continue
		}
		for _, merr := range manifestErrs {
			le.WithError(merr).Warn("ignoring invalid manifest")
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

		// Diff against previous value.
		next := &manifest.FetchManifestValue{ManifestRefs: refs}
		if entry.emitted == nil || !next.EqualVT(entry.emitted) {
			_ = entry.handler.ClearValues()
			_, _ = entry.handler.AddValue(next)
			entry.emitted = next
			le.WithField("manifest-id", mid).Debugf("resolved %d manifest(s)", len(manifests))
		}
		entry.handler.MarkIdle(true)
		trace.Log(entryCtx, "result", "resolved")
		entryTask.End()
	}
}
