package plugin_host_scheduler

import (
	"context"
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

// execWatchWorldManifest watches the world for the latest executable
// manifest for the plugin.
func (t *pluginInstance) execWatchWorldManifest(ctx context.Context, hosts *pluginHostSet) error {
	// Build the world engine and watch loop for the plugin host object.
	t.le.Debugf("starting watch world manifests")
	engineID := t.c.conf.GetEngineId()
	engine := world.NewBusEngine(ctx, t.c.bus, engineID)
	ws := world.NewEngineWorldState(engine, false)
	objLoop := world_control.NewWatchLoop(
		t.le.WithFields(logrus.Fields{
			"object-loop":        "watch-world-manifest",
			"engine-id":          engineID,
			"plugin-host-objkey": t.c.objKey,
		}),
		t.c.objKey,
		func(ctx context.Context, le *logrus.Entry, _ world.WorldState, obj world.ObjectState, _ *bucket.ObjectRef, _ uint64) (waitForChanges bool, err error) {
			// Share one read transaction across graph traversal and candidate reads.
			// Release it before the watch loop waits for another object revision.
			var missing []*bldr_manifest_world.StartupManifestCandidateEligibility
			err = world.ExecTransaction(ctx, engine, false, func(ctx context.Context, snapshot world.WorldState) error {
				var selectErr error
				waitForChanges, missing, selectErr = t.processManifestWorldState(ctx, le, hosts, snapshot, obj)
				return selectErr
			})
			if err != nil || len(missing) == 0 {
				return waitForChanges, err
			}

			// Unlink manifests whose blocks are gone so later loads skip them.
			err = world.ExecTransaction(ctx, engine, true, func(ctx context.Context, tx world.WorldState) error {
				unlinked, err := bldr_manifest_world.UnlinkMissingStartupManifests(ctx, tx, missing, t.c.objKey)
				if len(unlinked) != 0 {
					le.WithField("manifest-refs", unlinked).Info("unlinked manifest refs with missing blocks")
				}
				return err
			})
			return waitForChanges, err
		},
	)

	return objLoop.Execute(ctx, ws)
}

// processManifestWorldStateCore selects eligible manifests from one World snapshot.
func (t *pluginInstance) processManifestWorldStateCore(
	ctx context.Context,
	le *logrus.Entry,
	hosts *pluginHostSet,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
) (waitForChanges bool, missing []*bldr_manifest_world.StartupManifestCandidateEligibility, err error) {
	// Warn and wait for changes when the host object is missing.
	if obj == nil {
		le.Warnf("plugin host object not found: %v", t.c.objKey)
		return true, nil, nil
	}

	// Trace manifest selection and log accounting fields.
	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/select-manifest")
	defer task.End()
	t.logPluginAccountingFields(ctx)
	trace.Log(ctx, "host-object-key", t.c.objKey)

	// Wrap the world state with a verbose logger when configured.
	if t.c.conf.GetVerbose() {
		ws = world_vlogger.NewWorldState(le, ws)
	}

	// Collect the plugin's linked manifests for currently available hosts.
	// Build the sorted platform id list for the available hosts.
	platformIDsMap := t.platformHosts(hosts)
	platformIDs := sortedPlatformIDs(platformIDsMap)
	trace.Log(ctx, "platform-ids", strings.Join(platformIDs, ","))

	// Collect startup manifest eligibility for those platforms at the root.
	candidateEligibility, err := bldr_manifest_world.CollectStartupManifestEligibilityAtRoot(
		ctx,
		ws,
		t.pluginID,
		t.manifestRoot,
		platformIDs, // Collect for available platform ids
		t.c.objKey,
	)
	if err != nil {
		return true, nil, err
	}

	// Hold the first selection until FetchManifest settles, so startup runs
	// the announced release instead of the cached one it would replace. An
	// announced candidate with missing blocks still counts as linked.
	if t.awaitingFetch(candidateEligibility) {
		trace.Log(ctx, "manifest-selection-phase", "awaiting-fetch")
		return true, nil, nil
	}

	// Set aside candidates whose blocks are gone; the caller unlinks them.
	candidateEligibility = slices.DeleteFunc(candidateEligibility, func(c *bldr_manifest_world.StartupManifestCandidateEligibility) bool {
		if c.Missing {
			missing = append(missing, c)
		}
		return c.Missing
	})

	// Skip re-selection when the inputs match the stored fingerprint.
	selectionFingerprint := manifestSelectionInputFingerprint(platformIDs, candidateEligibility)
	if t.manifestSelectionInputUnchanged(hosts, selectionFingerprint) {
		trace.Log(ctx, "manifest-selection-phase", "skipped-unchanged-inputs")
		return true, missing, nil
	}
	if ctx.Err() != nil {
		return true, missing, context.Canceled
	}

	// Filter the selectable manifests down to compatible ones.
	manifests := bldr_manifest_world.SelectableStartupManifests(candidateEligibility)
	selectable := len(manifests)
	manifests = slices.DeleteFunc(manifests, func(m *bldr_manifest_world.CollectedManifest) bool {
		return t.incompatibleManifest(m.ManifestRef)
	})

	// Log candidate counts and summarize skipped candidates.
	trace.Logf(ctx, "candidate-count", "%d", len(candidateEligibility))
	trace.Logf(ctx, "selectable-candidate-count", "%d", len(manifests))
	trace.Logf(ctx, "skipped-candidate-count", "%d", countStartupManifestEligibilitySkips(candidateEligibility))
	skipSummary := summarizeStartupManifestEligibilitySkips(candidateEligibility)

	// Warn with a graph dump when candidates were skipped.
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
	if selectable != 0 && len(manifests) == 0 {
		// Every candidate failed startup on this host: keep any admitted worker
		// and its terminal status until the World offers another manifest.
		le.Warn("every selectable manifest is incompatible with this plugin host")
		t.storeManifestSelectionInputFingerprint(hosts, selectionFingerprint)
		return true, missing, nil
	}
	if len(manifests) == 0 {
		if t.manifestRoot != "" {
			t.finishInitialCapabilityRegistration(false)
		}
		t.storeManifestSelectionInputFingerprint(hosts, selectionFingerprint)
		t.c.recordPluginManifestRecoveryStatus(t.pluginID, t.instanceKey, nil, nil, candidateEligibility)
		// When store is disabled, the fetch handler may drive
		// execute/download directly from fetched ManifestRefs.
		// Don't clear states that the fetch handler set.
		if t.manifestRoot != "" || !t.c.conf.GetDisableStoreManifest() {
			_, changed1, _, _ := t.downloadManifestRoutine.SetState(nil)
			changed2 := t.setExecutePluginState(nil)
			if changed1 || changed2 || !t.loggedNotFound.Swap(true) {
				le.Debugf("no manifests for plugin found in world")
			}
		} else if !t.loggedNotFound.Swap(true) {
			le.Debugf("no manifests for plugin in world (store disabled, fetch may provide)")
		}
		return true, missing, nil
	}

	// Sort manifests by platform preference, revision, and ref string.
	slices.SortFunc(manifests, func(a, b *bldr_manifest_world.CollectedManifest) int {
		// Rank by platform preference and whether a host runs it.
		aPlatformID, bPlatformID := a.Manifest.GetMeta().GetPlatformId(), b.Manifest.GetMeta().GetPlatformId()
		aRank := candidateRank(aPlatformID, platformIDsMap[aPlatformID])
		bRank := candidateRank(bPlatformID, platformIDsMap[bPlatformID])
		if aRank != bRank {
			return aRank - bRank
		}

		// Prefer the newer revision, then the greater ref string.
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

	// Resolve locality against the same World snapshot used for selection.
	return true, missing, ws.AccessWorldState(
		ctx,
		nil,
		func(bls *bucket_lookup.Cursor) error {
			// The World bucket determines which candidates need a local copy.
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
				// The platform must still be part of this selection's set.
				manifestPlatformID := manifest.Manifest.GetMeta().GetPlatformId()
				manifestPluginHost, ok := platformIDsMap[manifestPlatformID]
				if !ok {
					continue
				}

				// Empty bucket IDs refer to this World's storage.
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

				// Keep the reference and metadata together through admission.
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

				// Copy the highest-ranked remote candidate while retaining a
				// usable local generation farther down the ordering.
				if downloadManifest == nil {
					downloadManifest = manifestSnapshot
					downloadManifestHost = manifestPluginHost
				}

			}

			// With no local candidate, execution can demand-load the remote one.
			if executeManifest == nil {
				executeManifest = downloadManifest
				executeManifestHost = downloadManifestHost
			}

			// Retain a still-selectable generation across late same-revision
			// platform arrivals. A missing candidate or replaced host cannot pin
			// execution, and newer revisions still follow the normal copy policy.
			if executeManifest != nil {
				currentState := t.executePluginRoutine.GetState()
				best := &manifestCandidate{
					ref:  bldr_manifest.NewManifestRef(executeManifest.GetManifest().GetMeta(), executeManifest.GetManifestRef()),
					host: executeManifestHost,
				}
				for _, manifest := range manifests {
					candidate := &manifestCandidate{
						ref:  bldr_manifest.NewManifestRef(manifest.Manifest.GetMeta(), manifest.ManifestRef),
						host: platformIDsMap[manifest.Manifest.GetMeta().GetPlatformId()],
					}
					if !candidate.matchesState(currentState) || !candidate.shouldRemainCurrent(best) {
						continue
					}
					retained := &bldr_manifest.ManifestSnapshot{
						ManifestRef: manifest.ManifestRef,
						Manifest:    manifest.Manifest,
					}
					if downloadManifest == executeManifest {
						downloadManifest = retained
					}
					executeManifest = retained
					executeManifestHost = candidate.host
					break
				}
			}

			// Record the copy accounting for the selected execute manifest.
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

			// Clear the not-found flag and record recovery status.
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

			// Apply the execute routine state for the selected manifest.
			var anyChanged bool

			// The routine container owns generation replacement and cancellation.
			if executeManifest != nil {
				changed := t.setExecutePluginState(&executePluginArgs{
					manifestSnapshot: executeManifest,
					pluginHost:       executeManifestHost,
					serveAssets:      executeManifestHost == nil,
				})
				anyChanged = anyChanged || changed
			} else {
				changed := t.setExecutePluginState(nil)
				anyChanged = anyChanged || changed
			}

			// Schedule the full-DAG local copy after the execute path so startup
			// demand fetches get the first chance at worker and shell blocks.
			// A nil or source-suppressed manifest clears the copy routine.
			changed := t.setDownloadManifestState(ctx, downloadManifest, worldBucketID)
			anyChanged = anyChanged || changed

			// Log the new selection when any routine state changed.
			if anyChanged {
				fields := logrus.Fields{}
				addManifestSelectionFields(fields, "download", downloadManifest)
				addManifestSelectionFields(fields, "execute", executeManifest)
				le.WithFields(fields).Debug("selected download and execute manifests for plugin")
			}

			// Persist the selection input fingerprint for the next pass.
			t.storeManifestSelectionInputFingerprint(hosts, selectionFingerprint)
			return nil
		},
	)
}

// manifestSelectionInput is the fingerprinted selection state used to
// suppress redundant re-selections.
type manifestSelectionInput struct {
	// hostSet binds the fingerprint to execution-host identity.
	hostSet *pluginHostSet
	// fingerprint records the eligible World candidates.
	fingerprint string
}

// manifestSelectionInputUnchanged reports whether the selection inputs
// are unchanged since the last fingerprint.
func (t *pluginInstance) manifestSelectionInputUnchanged(hosts *pluginHostSet, fingerprint string) bool {
	current := t.manifestSelectionFingerprint.Load()
	return current != nil &&
		current.hostSet == hosts &&
		current.fingerprint == fingerprint
}

// storeManifestSelectionInputFingerprint persists the current selection
// input fingerprint.
func (t *pluginInstance) storeManifestSelectionInputFingerprint(hosts *pluginHostSet, fingerprint string) {
	t.manifestSelectionFingerprint.Store(&manifestSelectionInput{
		hostSet:     hosts,
		fingerprint: fingerprint,
	})
}

// manifestSelectionInputFingerprint computes the fingerprint over the
// current selection inputs.
func manifestSelectionInputFingerprint(
	platformIDs []string,
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
) string {
	// Build the fingerprint with a length-prefixed field writer.
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

// summarizeStartupManifestEligibilitySkips builds a summary line of
// skipped candidate counts by eligibility kind.
func summarizeStartupManifestEligibilitySkips(candidates []*bldr_manifest_world.StartupManifestCandidateEligibility) string {
	// Return an empty summary with no candidates.
	if len(candidates) == 0 {
		return ""
	}

	// Collect summaries for the first few skipped candidates.
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

	// Join the summaries and append the remaining count.
	summary := strings.Join(items, "; ")
	if remaining := countStartupManifestEligibilitySkips(candidates) - len(items); remaining > 0 {
		summary += "; +" + strconv.Itoa(remaining) + " more"
	}
	return strconv.Itoa(countStartupManifestEligibilitySkips(candidates)) + " skipped startup manifest ref(s): " + summary
}

// countStartupManifestEligibilitySkips counts skipped candidates by
// eligibility kind.
func countStartupManifestEligibilitySkips(candidates []*bldr_manifest_world.StartupManifestCandidateEligibility) int {
	var count int
	for _, candidate := range candidates {
		if startupManifestEligibilitySkipCandidate(candidate) {
			count++
		}
	}
	return count
}

// startupManifestEligibilitySkipCandidate wraps a candidate with its
// skip reason for summary counting.
func startupManifestEligibilitySkipCandidate(candidate *bldr_manifest_world.StartupManifestCandidateEligibility) bool {
	if candidate == nil {
		return false
	}
	return candidate.Eligibility == bldr_manifest_world.StartupManifestEligibilityUnsafe ||
		candidate.Eligibility == bldr_manifest_world.StartupManifestEligibilityQuarantined
}

// addManifestSelectionFields adds manifest selection fields to a logrus
// entry.
func addManifestSelectionFields(
	fields logrus.Fields,
	prefix string,
	manifest *bldr_manifest.ManifestSnapshot,
) {
	// Fill in a none marker for a missing manifest.
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
