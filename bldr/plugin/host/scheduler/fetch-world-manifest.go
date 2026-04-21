package plugin_host_scheduler

import (
	"context"
	"sync"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/sirupsen/logrus"
)

type storeFetchedManifestsKey struct {
	valueID  uint32
	refIndex int
}

type directFetchCandidate struct {
	ref  *bldr_manifest.ManifestRef
	host bldr_plugin_host.PluginHost
}

// execute executes the tracker.
func (t *pluginInstance) execFetchWorldManifest(ctx context.Context, hosts *pluginHostSet) error {
	// wait for hosts set
	if hosts == nil {
		return nil
	}

	platformIDs := hosts.toPlatformIDs()
	t.le.
		WithField("platform-ids", platformIDs).
		Debugf("starting fetch plugin manifests")

	// If configured, store manifests in the world.
	storeManifests := !t.c.conf.GetDisableStoreManifest()
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
		handler = t.newDirectFetchHandler(hosts)
	}

	_, ref, err := t.c.bus.AddDirective(
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

	// we are done
	_ = context.AfterFunc(ctx, ref.Release)
	return nil
}

type fetchManifestValueStorer struct {
	pi     *pluginInstance
	value  *promise.Promise[*bldr_manifest.FetchManifestValue]
	refIdx int
}

func (t *pluginInstance) newManifestFetchValueStorer(key storeFetchedManifestsKey) (keyed.Routine, *fetchManifestValueStorer) {
	s := &fetchManifestValueStorer{pi: t, refIdx: key.refIndex}
	s.value = promise.NewPromise[*bldr_manifest.FetchManifestValue]()
	return s.execFetchManifestValueStorer, s
}

// execFetchManifestValueStorer executes storing the FetchManifest value in storage.
func (t *fetchManifestValueStorer) execFetchManifestValueStorer(ctx context.Context) error {
	fetchManifestValue, err := t.value.Await(ctx)
	if err != nil {
		return err
	}

	manifestRefs := fetchManifestValue.GetManifestRefs()
	if len(manifestRefs) <= t.refIdx {
		return nil
	}

	manifestRef := manifestRefs[t.refIdx]
	meta := manifestRef.GetMeta()
	le := meta.Logger(t.pi.le)
	le.Debug("downloading and storing plugin manifest ref")

	ws, err := t.pi.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	manifestKey := bldr_manifest.NewManifestKey(t.pi.c.objKey, meta)
	prevManifest, prevManifestFound, err := ws.GetObject(ctx, manifestKey)
	if err != nil {
		return err
	}
	var prevManifestRootRef *bucket.ObjectRef
	if prevManifest != nil {
		prevManifestRootRef, _, err = prevManifest.GetRootRef(ctx)
		if err != nil {
			return err
		}
	}

	// TODO: See manifest/builder/controller/controller.go
	// TODO: We don't increment the manifest revision in devtool mode when hot reloading.
	// TODO: Instead make sure the root ref is the same in storage here.
	if prevManifestFound && prevManifestRootRef.EqualVT(manifestRef.GetManifestRef()) {
		// manifest exists, do nothing.
		le.Debug("manifest is identical, skipping")
		return nil
	}

	// detects changes and does nothing if there are no changes
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

	le.Info("successfully fetched and stored manifest ref")
	return nil
}

// newDirectFetchHandler builds a handler that drives execute/download directly
// from fetched ManifestRefs when store is disabled (e.g., Space plugins in devtool mode).
func (t *pluginInstance) newDirectFetchHandler(hosts *pluginHostSet) directive.ReferenceHandler {
	var mtx sync.Mutex
	allRefs := make(map[uint32][]*bldr_manifest.ManifestRef)
	platformIDsMap := hosts.toPlatformIDsMap()

	selectBest := func() {
		var best *directFetchCandidate
		var current *directFetchCandidate
		currentState := t.executePluginRoutine.GetState()
		for _, refs := range allRefs {
			for _, ref := range refs {
				meta := ref.GetMeta()
				host, ok := platformIDsMap[meta.GetPlatformId()]
				if !ok || host == nil {
					continue
				}
				candidate := &directFetchCandidate{
					ref:  ref,
					host: host,
				}
				if current == nil && directFetchCandidateMatchesState(candidate, currentState) {
					current = candidate
				}
				if best == nil || directFetchCandidateBetter(candidate, best) {
					best = candidate
				}
			}
		}

		if current != nil && (best == nil || current.ref.GetMeta().GetRev() >= best.ref.GetMeta().GetRev()) {
			best = current
		}

		if best != nil {
			snapshot := &bldr_manifest.ManifestSnapshot{
				ManifestRef: best.ref.GetManifestRef(),
			}
			if !t.c.conf.GetDisableCopyManifest() {
				t.downloadManifestRoutine.SetState(snapshot)
			}
			t.executePluginRoutine.SetState(&executePluginArgs{
				manifestSnapshot: snapshot,
				pluginHost:       best.host,
			})
			t.loggedNotFound.Store(false)
			return
		}

		if len(allRefs) == 0 &&
			(t.executePluginRoutine.GetState() != nil || t.downloadManifestRoutine.GetState() != nil) {
			t.le.Debug("preserving current plugin target while fetched manifest refs are temporarily empty")
			return
		}

		t.executePluginRoutine.SetState(nil)
		if !t.c.conf.GetDisableCopyManifest() {
			t.downloadManifestRoutine.SetState(nil)
		}
	}

	return directive.NewTypedCallbackHandler(
		func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
			mtx.Lock()
			defer mtx.Unlock()
			refs := av.GetValue().GetManifestRefs()
			validRefs := make([]*bldr_manifest.ManifestRef, 0, len(refs))
			for _, ref := range refs {
				if err := ref.Validate(); err != nil {
					t.le.WithError(err).Warn("skipping invalid manifest ref")
					continue
				}
				validRefs = append(validRefs, ref)
			}
			allRefs[av.GetValueID()] = validRefs
			selectBest()
		},
		func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
			mtx.Lock()
			defer mtx.Unlock()
			delete(allRefs, av.GetValueID())
			selectBest()
		},
		nil, nil,
	)
}

func directFetchCandidateBetter(candidate, current *directFetchCandidate) bool {
	if current == nil {
		return true
	}

	candidateRev := candidate.ref.GetMeta().GetRev()
	currentRev := current.ref.GetMeta().GetRev()
	if candidateRev != currentRev {
		return candidateRev > currentRev
	}

	candidateRef := candidate.ref.String()
	currentRef := current.ref.String()
	if candidateRef != currentRef {
		return candidateRef > currentRef
	}

	return candidate.host.GetPlatformId() > current.host.GetPlatformId()
}

func directFetchCandidateMatchesState(candidate *directFetchCandidate, currentState *executePluginArgs) bool {
	if currentState == nil || currentState.pluginHost != candidate.host {
		return false
	}
	if currentState.manifestSnapshot == nil || currentState.manifestSnapshot.GetManifestRef() == nil {
		return false
	}

	return currentState.manifestSnapshot.GetManifestRef().EqualVT(candidate.ref.GetManifestRef())
}
