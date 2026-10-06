package plugin_host_scheduler

import (
	"context"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/sirupsen/logrus"
)

// storeFetchedManifestsKey identifies one reference in a fetched value.
type storeFetchedManifestsKey struct {
	// valueID identifies the attached fetch result.
	valueID uint32
	// refIndex selects the manifest within that result.
	refIndex int
}

// execFetchWorldManifestCore fetches plugin manifests via FetchManifest and
// feeds the store and execute paths.
func (t *pluginInstance) execFetchWorldManifestCore(ctx context.Context, hosts *pluginHostSet) (rerr error) {
	// Record any fetch failure in the plugin status.
	defer func() {
		t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "fetch plugin manifest", rerr)
	}()

	// Wait until an execution host is available.
	if hosts == nil {
		return nil
	}

	// Open the fetch-manifest trace task and log the plugin accounting fields.
	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/fetch-manifest")
	defer task.End()
	t.logPluginAccountingFields(ctx)

	// Collect the platform IDs this plugin can select and log them.
	platformIDs := sortedPlatformIDs(t.platformHosts(hosts))
	trace.Log(ctx, "platform-ids", strings.Join(platformIDs, ","))
	t.le.
		WithField("platform-ids", platformIDs).
		Debugf("starting fetch plugin manifests")

	// Build the directive handler: a World store handler when manifests are
	// stored, otherwise a direct fetch handler.
	storeManifests := !t.c.conf.GetDisableStoreManifest()
	trace.Logf(ctx, "store-manifests", "%t", storeManifests)
	var handler directive.ReferenceHandler
	if storeManifests {
		// Keyed set of FetchManifestValue store routines.
		storeFetchedManifests := keyed.NewKeyedWithLogger(
			t.newManifestFetchValueStorer,
			t.le,
			keyed.WithRetry[storeFetchedManifestsKey, *fetchManifestValueStorer](&backoff.Backoff{}),
		)
		storeFetchedManifests.SetContext(ctx, false)

		handler = directive.NewTypedCallbackHandler(
			// value added
			func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
				manifestValue := av.GetValue()
				for i, manifestRef := range manifestValue.GetManifestRefs() {
					if err := manifestRef.Validate(); err != nil {
						t.le.WithError(err).Warn("skipping invalid manifest ref")
						continue
					}

					storer, _ := storeFetchedManifests.SetKey(storeFetchedManifestsKey{valueID: av.GetValueID(), refIndex: i}, true)
					storer.value.SetResult(av.GetValue(), nil)
				}
			},
			// value removed
			func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
				for _, key := range storeFetchedManifests.GetKeys() {
					if key.valueID == av.GetValueID() {
						storeFetchedManifests.RemoveKey(key)
					}
				}
			},
			// disposed (ignore, only happens once ctx cancels)
			nil, // func() {},
			nil,
		)
	} else {
		handler = t.newDirectFetchHandler(ctx, hosts)
	}

	// Add the FetchManifest directive for this plugin and platform set.
	di, ref, err := t.c.bus.AddDirective(
		bldr_manifest.NewFetchManifest(
			// use pluginID as manifest id
			t.pluginID,
			// accept any build type (presuming we only have the right one)
			nil,
			// accept any of the platform ids we have
			platformIDs,
			// accept any revision
			0,
		),
		handler,
	)
	if err != nil {
		return err
	}

	// Record each directive idle error in the plugin status.
	releaseIdle := di.AddIdleCallback(func(isIdle bool, errs []error) {
		if !isIdle {
			return
		}
		for _, err := range errs {
			t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "fetch plugin manifest", err)
		}
	})

	// Keep the fetch reference alive until this host selection is canceled.
	_ = context.AfterFunc(ctx, func() {
		releaseIdle()
		ref.Release()
	})
	return nil
}

// fetchManifestValueStorer stores fetched manifest values on the plugin
// instance.
type fetchManifestValueStorer struct {
	// pi owns registration and its status.
	pi *pluginInstance
	// value supplies the immutable fetched references.
	value *promise.Promise[*bldr_manifest.FetchManifestValue]
	// valueID identifies the attached result for tracing.
	valueID uint32
	// refIdx selects the reference registered by this routine.
	refIdx int
}

// newManifestFetchValueStorer constructs a value storer bound to this
// instance and key.
func (t *pluginInstance) newManifestFetchValueStorer(key storeFetchedManifestsKey) (keyed.Routine, *fetchManifestValueStorer) {
	// Allocate the storer with a promise for its fetched value.
	s := &fetchManifestValueStorer{pi: t, valueID: key.valueID, refIdx: key.refIndex}
	s.value = promise.NewPromise[*bldr_manifest.FetchManifestValue]()
	return s.execFetchManifestValueStorer, s
}

// execFetchManifestValueStorerCore registers a fetched reference in the World.
func (t *fetchManifestValueStorer) execFetchManifestValueStorerCore(ctx context.Context) (rerr error) {
	// Record any store failure in the plugin status.
	defer func() {
		t.pi.c.recordPluginStatusError(
			t.pi.pluginID,
			t.pi.instanceKey,
			"store fetched plugin manifest",
			rerr,
		)
	}()

	// Open the store-result trace task and log the fetch accounting fields.
	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/fetch-manifest/store-result")
	defer task.End()
	t.pi.logPluginAccountingFields(ctx)
	trace.Logf(ctx, "fetch-value-id", "%d", t.valueID)
	trace.Logf(ctx, "fetch-ref-index", "%d", t.refIdx)

	// Await the fetched manifest value this storer was created for.
	fetchManifestValue, err := t.value.Await(ctx)
	if err != nil {
		return err
	}

	// Stop when this storer's reference index is beyond the fetched refs.
	manifestRefs := fetchManifestValue.GetManifestRefs()
	if len(manifestRefs) <= t.refIdx {
		return nil
	}

	// Log the manifest reference this storer registers.
	manifestRef := manifestRefs[t.refIdx]
	logManifestRefAccountingFields(ctx, "fetched", manifestRef)
	meta := manifestRef.GetMeta()
	le := meta.Logger(t.pi.le)
	le.Debug("downloading and storing plugin manifest ref")

	// Wait for the plugin host's World state.
	ws, err := t.pi.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Register the fetched manifest reference in the World.
	manifestKey := bldr_manifest.NewManifestArtifactKey(manifestRef.GetManifestRef())

	// Registration updates the host only when its manifest reference changes.
	le.
		WithFields(logrus.Fields{
			"manifest-ref": manifestRef.GetManifestRef().String(),
			"host-obj-key": t.pi.c.objKey,
		}).
		Debug("registering fetched plugin manifest ref")
	err = bldr_manifest_world.ExStoreManifestOp(
		ctx,
		ws,
		t.pi.c.peerID,
		manifestKey,
		[]string{t.pi.c.objKey},
		manifestRef,
	)
	if err != nil {
		return err
	}

	// Log the successful registration.
	le.Info("successfully fetched and stored manifest ref")
	return nil
}

// newDirectFetchHandler builds a handler that drives execute/download directly
// from fetched canonical ManifestRefs when store is disabled (e.g., Space
// plugins in devtool mode).
func (t *pluginInstance) newDirectFetchHandler(ctx context.Context, hosts *pluginHostSet) directive.ReferenceHandler {
	// Hold the fetched references per value ID and the platform host mapping.
	var mtx sync.Mutex
	allRefs := make(map[uint32][]*bldr_manifest.ManifestRef)
	platformIDsMap := t.platformHosts(hosts)

	// Re-select the best compatible manifest candidate from all fetched refs.
	selectBest := func() {
		// Start with no best, current, or rejected candidates.
		var best *manifestCandidate
		var current *manifestCandidate
		var rejected bool

		// Score every fetched reference against the hosts and the running
		// plugin state.
		currentState := t.executePluginRoutine.GetState()
		for _, refs := range allRefs {
			for _, ref := range refs {
				meta := ref.GetMeta()
				host, ok := platformIDsMap[meta.GetPlatformId()]
				if !ok {
					continue
				}
				if t.incompatibleManifest(ref.GetManifestRef()) {
					rejected = true
					continue
				}
				candidate := &manifestCandidate{
					ref:  ref,
					host: host,
				}
				if current == nil && candidate.matchesState(currentState) {
					current = candidate
				}
				if best == nil || candidate.betterThan(best) {
					best = candidate
				}
			}
		}

		// Keep the current candidate when it still matches the best one.
		if current != nil && current.shouldRemainCurrent(best) {
			best = current
		}

		// Apply the best candidate to the execute and download routines.
		if best != nil {
			snapshot := &bldr_manifest.ManifestSnapshot{
				ManifestRef: best.ref.GetManifestRef(),
			}
			t.setExecutePluginState(&executePluginArgs{
				manifestSnapshot: snapshot,
				pluginHost:       best.host,
				serveAssets:      best.host == nil,
			})
			t.setDownloadManifestState(ctx, snapshot, t.c.conf.GetEngineId())
			t.loggedNotFound.Store(false)
			return
		}

		// Preserve the current target when no compatible ref was fetched but
		// a plugin is already running or downloading.
		if (len(allRefs) == 0 || rejected) &&
			(t.executePluginRoutine.GetState() != nil || t.downloadManifestRoutine.GetState() != nil) {
			t.le.Debug("preserving current plugin target without a compatible fetched manifest ref")
			return
		}

		// Clear the execute and download targets.
		t.setExecutePluginState(nil)
		t.setDownloadManifestState(ctx, nil, t.c.conf.GetEngineId())
	}

	// Return the handler that canonicalizes fetched refs and re-selects.
	return directive.NewTypedCallbackHandler(
		// value added
		func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
			// Store the canonicalized refs for this value and re-select.
			mtx.Lock()
			defer mtx.Unlock()
			refs := av.GetValue().GetManifestRefs()
			validRefs := make([]*bldr_manifest.ManifestRef, 0, len(refs))
			for _, ref := range refs {
				if err := ref.Validate(); err != nil {
					t.le.WithError(err).Warn("skipping invalid manifest ref")
					continue
				}
				canonicalRef, err := bldr_manifest_world.CanonicalizeManifestObjectRef(ctx, nil, ref.GetManifestRef())
				if err != nil {
					ws, waitErr := t.c.worldStateCtr.WaitValue(ctx, nil)
					if waitErr != nil {
						t.le.WithError(waitErr).Warn("skipping fetched manifest ref without world state")
						continue
					}
					canonicalRef, err = bldr_manifest_world.CanonicalizeManifestObjectRef(ctx, ws.AccessWorldState, ref.GetManifestRef())
				}
				if err != nil {
					t.le.WithError(err).Warn("skipping fetched manifest ref with unresolved transform config")
					continue
				}
				validRefs = append(validRefs, bldr_manifest.NewManifestRef(ref.GetMeta(), canonicalRef))
			}
			allRefs[av.GetValueID()] = validRefs
			selectBest()
		},
		// value removed
		func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
			// Drop this value's refs and re-select.
			mtx.Lock()
			defer mtx.Unlock()
			delete(allRefs, av.GetValueID())
			selectBest()
		},
		nil, nil,
	)
}
