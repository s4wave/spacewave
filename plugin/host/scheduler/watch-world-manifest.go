package plugin_host_scheduler

import (
	"context"
	"maps"
	"slices"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_vlogger "github.com/aperturerobotics/hydra/world/vlogger"
	"github.com/sirupsen/logrus"
)

// execute executes the tracker.
func (t *pluginInstance) execWatchWorldManifest(ctx context.Context, hosts *pluginHostSet) error {
	t.le.Debugf("starting watch world manifests")
	engineID := t.c.conf.GetEngineId()
	objLoop := world_control.NewWatchLoop(
		t.le.WithFields(logrus.Fields{
			"object-loop":        "watch-world-manifest",
			"engine-id":          engineID,
			"plugin-host-objkey": t.c.objKey,
		}),
		t.c.objKey,
		func(ctx context.Context, le *logrus.Entry, ws world.WorldState, obj world.ObjectState, _ *bucket.ObjectRef, _ uint64) (waitForChanges bool, err error) {
			return t.processManifestWorldState(ctx, le, hosts, ws, obj)
		},
	)

	return world_control.ExecuteBusWatchLoop(
		ctx,
		t.c.bus,
		engineID,
		false,
		objLoop,
	)
}

// processManifestWorldState processes the state for the PluginManifest.
func (t *pluginInstance) processManifestWorldState(
	ctx context.Context,
	le *logrus.Entry,
	hosts *pluginHostSet,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
) (waitForChanges bool, err error) {
	if obj == nil {
		le.Warnf("plugin host object not found: %v", t.c.objKey)
		return true, nil
	}

	// TODO remove
	ws = world_vlogger.NewWorldState(le, ws)

	// Lookup the latest PluginManifests matching our plugin linked to PluginHost.
	platformIDsMap := hosts.toPlatformIDsMap()
	platformIDs := slices.Collect(maps.Keys(platformIDsMap))
	slices.Sort(platformIDs)

	// configure logger
	le = le.WithFields(logrus.Fields{
		"platform-ids":    platformIDs,
		"host-object-key": t.c.objKey,
	})

	// collect manifests
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		t.pluginID,
		platformIDs, // Collect for available platform ids
		t.c.objKey,
	)
	if err != nil {
		return true, err
	}
	for _, manifestErr := range manifestErrs {
		le.WithError(manifestErr).Warn("skipping manifest due to error")
	}
	if len(manifests) == 0 {
		le.Infof("no manifests for plugin found in world")
		return true, nil
	}

	// sort by rev and platform id
	// the resulting slice will be sorted by ManifestID, then by Rev (descending), then by PlatformID.
	manifests = bldr_manifest_world.FilterCollectedManifestsByLatestRev(manifests)

	// return the result of this + true to keep waiting
	return true, ws.AccessWorldState(
		ctx,
		// access the root of the world state
		nil,
		func(bls *bucket_lookup.Cursor) error {
			// get the bucket id of the world state
			worldBucketID := bls.GetOpArgs().GetBucketId()

			// decide the "download manifest" and the "execute manifest" based on which is fully downloaded
			// we consider a manifest to be fully downloaded if its ref bucket matches the world bucket
			// this way we will fully download the manifest(s) before swapping in a new version
			var downloadManifest, executeManifest *bldr_manifest.ManifestSnapshot
			var downloadManifestHost, executeManifestHost plugin_host.PluginHost

			// prefer the manifest with highest revision and corresponding plugin host
			// the slice is sorted this way
			for _, manifest := range manifests {
				// find the corresponding plugin host
				manifestPlatformID := manifest.Manifest.GetMeta().GetPlatformId()
				manifestPluginHost, ok := platformIDsMap[manifestPlatformID]
				if !ok || manifestPluginHost == nil {
					// if no plugin host found, continue
					// this shouldn't happen since we filtered by platformIDs above
					continue
				}

				// check if the manifest bucket id is within the same world bucket
				le := manifest.Manifest.GetMeta().Logger(le)
				manifestBucketID := manifest.ManifestRef.GetBucketId()
				if manifestBucketID == "" {
					le.Warn("bucket id in manifest root ref is empty, assuming world bucket")
					manifestBucketID = worldBucketID
					manifest.ManifestRef.BucketId = worldBucketID
				}

				// needs download if bucket id differs
				needsDownload := manifestBucketID != worldBucketID

				// create the snapshot
				manifestSnapshot := &bldr_manifest.ManifestSnapshot{
					ManifestRef: manifest.ManifestRef,
					Manifest:    manifest.Manifest,
				}

				if !needsDownload {
					// we have our downloaded manifest to execute.
					executeManifest = manifestSnapshot
					executeManifestHost = manifestPluginHost
					break
				}

				// set downloadManifest = manifestSnapshot if we don't already have a downloadManifest
				if downloadManifest == nil {
					downloadManifest = manifestSnapshot
					downloadManifestHost = manifestPluginHost
				}

				// keep looking for a candidate to execute
				continue
			}

			// if we have no candidate to execute use downloadManifest
			if executeManifest == nil {
				executeManifest = downloadManifest
				executeManifestHost = downloadManifestHost
			}

			// download the downloadManifest
			// if downloadManifest is nil this will stop that routine
			var anyChanged bool
			if !t.c.conf.GetDisableCopyManifest() {
				_, changed, _, _ := t.downloadManifestRoutine.SetState(downloadManifest)
				anyChanged = anyChanged || changed
			}

			// execute the executeManifest
			if executeManifest != nil {
				// update the state container (which automatically diffs the manifest and restarts if changed)
				_, changed, _, _ := t.executePluginRoutine.SetState(&executePluginArgs{
					manifestSnapshot: executeManifest,
					pluginHost:       executeManifestHost,
				})
				anyChanged = anyChanged || changed
			} else {
				_, changed, _, _ := t.executePluginRoutine.SetState(nil)
				anyChanged = anyChanged || changed
			}

			if anyChanged {
				le.WithFields(logrus.Fields{
					"download-manifest-rev": downloadManifest.GetManifest().GetMeta().GetRev(),
					"download-manifest-ref": downloadManifest.GetManifestRef().MarshalB58(),
					"execute-manifest-ref":  executeManifest.GetManifestRef().MarshalB58(),
					"execute-manifest-rev":  executeManifest.GetManifest().GetMeta().GetRev(),
				}).Debug("selected download and execute manifests for plugin")
			}

			// done
			return nil
		},
	)
}
