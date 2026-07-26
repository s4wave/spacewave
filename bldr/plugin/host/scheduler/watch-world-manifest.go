package plugin_host_scheduler

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
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

	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/select-manifest")
	defer task.End()
	t.logPluginAccountingFields(ctx)
	trace.Log(ctx, "host-object-key", t.c.objKey)

	if t.c.conf.GetVerbose() {
		ws = world_vlogger.NewWorldState(le, ws)
	}

	// Lookup PluginManifests matching our plugin linked to PluginHost.
	platformIDsMap := hosts.toPluginPlatformIDsMap(t.c.conf, t.pluginID)
	platformIDs := slices.Collect(maps.Keys(platformIDsMap))
	slices.Sort(platformIDs)
	trace.Log(ctx, "platform-ids", strings.Join(platformIDs, ","))
	// collect and classify retained startup manifest candidates
	candidateEligibility, err := bldr_manifest_world.CollectStartupManifestEligibilityForManifestID(
		ctx,
		ws,
		t.pluginID,
		platformIDs, // Collect for available platform ids
		t.c.objKey,
	)
	if err != nil {
		return true, err
	}
	selectionFingerprint := manifestSelectionInputFingerprint(platformIDs, candidateEligibility)
	if t.manifestSelectionInputUnchanged(hosts, selectionFingerprint) {
		trace.Log(ctx, "manifest-selection-phase", "skipped-unchanged-inputs")
		return true, nil
	}
	if ctx.Err() != nil {
		return true, context.Canceled
	}
	manifests := bldr_manifest_world.SelectableStartupManifests(candidateEligibility)
	trace.Logf(ctx, "candidate-count", "%d", len(candidateEligibility))
	trace.Logf(ctx, "selectable-candidate-count", "%d", len(manifests))
	trace.Logf(ctx, "skipped-candidate-count", "%d", countStartupManifestEligibilitySkips(candidateEligibility))
	skipSummary := summarizeStartupManifestEligibilitySkips(candidateEligibility)
	if skipSummary != "" {
		logEntry := le.WithField("skipped-startup-manifest-refs", skipSummary)
		graphDump, dumpErr := bldr_manifest_world.DumpStartupManifestGraphForManifestID(
			ctx,
			ws,
			t.pluginID,
			platformIDs,
			t.c.objKey,
		)
		if dumpErr != nil {
			logEntry = logEntry.WithError(dumpErr)
		}
		if dumpErr == nil && graphDump != "" {
			logEntry = logEntry.WithField("startup-manifest-graph", graphDump)
		}
		logEntry.Warn("skipped startup manifest refs")
		t.c.recordPluginStatusError(
			t.pluginID,
			t.instanceKey,
			"startup manifest refs",
			errors.New(skipSummary),
		)
	} else {
		t.c.clearPluginStatusErrorStage(t.pluginID, t.instanceKey, "startup manifest refs")
	}
	if len(manifests) == 0 {
		t.storeManifestSelectionInputFingerprint(hosts, selectionFingerprint)
		t.c.recordPluginManifestRecoveryStatus(t.pluginID, t.instanceKey, nil, nil, candidateEligibility)
		// When store is disabled, the fetch handler may drive
		// execute/download directly from fetched ManifestRefs.
		// Don't clear states that the fetch handler set.
		if !t.c.conf.GetDisableStoreManifest() {
			_, changed1, _, _ := t.downloadManifestRoutine.SetState(nil)
			_, changed2, _, _ := t.executePluginRoutine.SetState(nil)
			if changed1 || changed2 || !t.loggedNotFound.Swap(true) {
				le.Debugf("no manifests for plugin found in world")
			}
		} else if !t.loggedNotFound.Swap(true) {
			le.Debugf("no manifests for plugin in world (store disabled, fetch may provide)")
		}
		return true, nil
	}
	slices.SortFunc(manifests, func(a, b *bldr_manifest_world.CollectedManifest) int {
		aRank := platformPreferenceRank(a.Manifest.GetMeta().GetPlatformId())
		bRank := platformPreferenceRank(b.Manifest.GetMeta().GetPlatformId())
		if aRank != bRank {
			return bRank - aRank
		}
		aRev := a.GetRev()
		bRev := b.GetRev()
		if aRev > bRev {
			return -1
		}
		if aRev < bRev {
			return 1
		}
		return strings.Compare(b.ManifestRef.String(), a.ManifestRef.String())
	})

	// return the result of this + true to keep waiting
	return true, ws.AccessWorldState(
		ctx,
		// access the root of the world state
		nil,
		func(bls *bucket_lookup.Cursor) error {
			// get the bucket id of the world state
			worldBucketID := bls.GetOpArgs().GetBucketId()
			trace.Log(ctx, "world-bucket-id", worldBucketID)

			// Select an execute manifest that is local or backed by an
			// authoritative no-copy bucket. Other external manifests must be
			// copied into the world bucket before execution switches to them.
			var downloadManifest, executeManifest *bldr_manifest.ManifestSnapshot
			var downloadManifestHost, executeManifestHost plugin_host.PluginHost

			// Prefer candidates in sorted order, but keep looking past external
			// copy candidates for an execute-eligible manifest.
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

				// Configured no-copy buckets remain authoritative without a
				// local DAG copy and are therefore execute-eligible.
				noCopy := slices.Contains(t.c.conf.GetNoCopyBucketIds(), manifestBucketID)
				needsDownload := manifestBucketID != worldBucketID

				// create the snapshot
				manifestSnapshot := &bldr_manifest.ManifestSnapshot{
					ManifestRef: manifest.ManifestRef,
					Manifest:    manifest.Manifest,
				}

				if !needsDownload || noCopy {
					executeManifest = manifestSnapshot
					executeManifestHost = manifestPluginHost
					if noCopy && downloadManifest == nil {
						downloadManifest = manifestSnapshot
						downloadManifestHost = manifestPluginHost
					}
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
			if executeManifest != nil {
				executeRef := executeManifest.GetManifestRef()
				sourceBucketID := ""
				if executeRef != nil {
					sourceBucketID = executeRef.GetBucketId()
				}
				t.setManifestCopySelection(ctx, executeManifest, sourceBucketID, worldBucketID)
				trace.Log(ctx, "manifest-selection-phase", "selected")
			} else {
				t.manifestCopyAccounting.Store(nil)
			}

			if executeManifest != nil || downloadManifest != nil {
				t.loggedNotFound.Store(false)
			}
			t.c.recordPluginManifestRecoveryStatus(
				t.pluginID,
				t.instanceKey,
				executeManifest,
				downloadManifest,
				candidateEligibility,
			)
			logManifestSnapshotAccountingFields(ctx, "execute", executeManifest)
			logManifestSnapshotAccountingFields(ctx, "download", downloadManifest)
			if downloadManifest != nil {
				trace.Log(ctx, "download-manifest-copy-class", string(t.classifyManifestCopy(downloadManifest)))
			}

			var anyChanged bool

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

			// Schedule the full-DAG local copy after the execute path so startup
			// demand fetches get the first chance at worker and shell blocks.
			// A nil or source-suppressed manifest clears the copy routine.
			changed := t.setDownloadManifestState(ctx, downloadManifest, worldBucketID)
			anyChanged = anyChanged || changed

			if anyChanged {
				fields := logrus.Fields{}
				addManifestSelectionFields(fields, "download", downloadManifest)
				addManifestSelectionFields(fields, "execute", executeManifest)
				le.WithFields(fields).Debug("selected download and execute manifests for plugin")
			}

			t.storeManifestSelectionInputFingerprint(hosts, selectionFingerprint)
			return nil
		},
	)
}

type manifestSelectionInput struct {
	hostSet     *pluginHostSet
	fingerprint string
}

func (t *pluginInstance) manifestSelectionInputUnchanged(hosts *pluginHostSet, fingerprint string) bool {
	current := t.manifestSelectionFingerprint.Load()
	return current != nil &&
		current.hostSet == hosts &&
		current.fingerprint == fingerprint
}

func (t *pluginInstance) storeManifestSelectionInputFingerprint(hosts *pluginHostSet, fingerprint string) {
	t.manifestSelectionFingerprint.Store(&manifestSelectionInput{
		hostSet:     hosts,
		fingerprint: fingerprint,
	})
}

func manifestSelectionInputFingerprint(
	platformIDs []string,
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
) string {
	var fingerprint strings.Builder
	writeField := func(value string) {
		fingerprint.WriteByte(0)
		fingerprint.WriteString(strconv.Itoa(len(value)))
		fingerprint.WriteByte(':')
		fingerprint.WriteString(value)
	}
	for _, platformID := range platformIDs {
		writeField(platformID)
	}
	candidates = slices.Clone(candidates)
	slices.SortStableFunc(candidates, func(a, b *bldr_manifest_world.StartupManifestCandidateEligibility) int {
		if a == nil || b == nil {
			if a == nil && b == nil {
				return 0
			}
			if a == nil {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ObjectKey, b.ObjectKey)
	})
	for _, candidate := range candidates {
		if candidate == nil {
			writeField("<nil>")
			continue
		}
		writeField(candidate.ObjectKey)
		writeField(candidate.EdgeLabel)
		writeField(string(candidate.Eligibility))
		writeField(candidate.Reason)
		writeField(candidate.ManifestID)
		writeField(candidate.PlatformID)
		writeField(strconv.FormatUint(candidate.Rev, 10))
		if candidate.ObjectRef != nil {
			writeField(candidate.ObjectRef.MarshalString())
		} else {
			writeField("<nil>")
		}
		if candidate.ManifestRef != nil {
			writeField(candidate.ManifestRef.String())
		} else {
			writeField("<nil>")
		}
		if candidate.Manifest != nil && candidate.Manifest.GetMeta() != nil {
			writeField(candidate.Manifest.GetMeta().MarshalB58())
		} else {
			writeField("<nil>")
		}
	}
	return fingerprint.String()
}

const maxStartupManifestSkipSummaryItems = 3

func summarizeStartupManifestEligibilitySkips(candidates []*bldr_manifest_world.StartupManifestCandidateEligibility) string {
	if len(candidates) == 0 {
		return ""
	}

	items := make([]string, 0, maxStartupManifestSkipSummaryItems)
	for _, candidate := range candidates {
		if len(items) >= maxStartupManifestSkipSummaryItems {
			break
		}
		if !startupManifestEligibilitySkipCandidate(candidate) {
			continue
		}
		items = append(items, candidate.Summary())
	}
	if len(items) == 0 {
		return ""
	}

	summary := strings.Join(items, "; ")
	if remaining := countStartupManifestEligibilitySkips(candidates) - len(items); remaining > 0 {
		summary += "; +" + strconv.Itoa(remaining) + " more"
	}
	return strconv.Itoa(countStartupManifestEligibilitySkips(candidates)) + " skipped startup manifest ref(s): " + summary
}

func countStartupManifestEligibilitySkips(candidates []*bldr_manifest_world.StartupManifestCandidateEligibility) int {
	var count int
	for _, candidate := range candidates {
		if startupManifestEligibilitySkipCandidate(candidate) {
			count++
		}
	}
	return count
}

func startupManifestEligibilitySkipCandidate(candidate *bldr_manifest_world.StartupManifestCandidateEligibility) bool {
	if candidate == nil {
		return false
	}
	return candidate.Eligibility == bldr_manifest_world.StartupManifestEligibilityUnsafe ||
		candidate.Eligibility == bldr_manifest_world.StartupManifestEligibilityQuarantined
}

func addManifestSelectionFields(
	fields logrus.Fields,
	prefix string,
	manifest *bldr_manifest.ManifestSnapshot,
) {
	if manifest == nil {
		fields[prefix+"-manifest"] = "none"
		return
	}
	ref := manifest.GetManifestRef()
	if ref == nil {
		fields[prefix+"-manifest-ref"] = "none"
	} else {
		fields[prefix+"-manifest-ref"] = ref.MarshalB58()
	}
	if manifest.GetManifest() == nil || manifest.GetManifest().GetMeta() == nil {
		return
	}
	fields[prefix+"-manifest-rev"] = manifest.GetManifest().GetMeta().GetRev()
}
