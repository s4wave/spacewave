package manifest_fetch_world

import (
	"context"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
)

// fetchManifestResolver resolves FetchManifest with the controller optionally watching for changes.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// dir is the FetchManifest directive
	dir manifest.FetchManifest
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	_ = handler.ClearValues()

	// Watch the world state and re-check the manifests list when it changes.
	le := r.c.le.WithField("engine-id", r.c.conf.GetEngineId()).WithField("manifest-id", r.dir.GetManifestId())
	le.Debug("starting watch world for manifest details")
	defer le.Debug("exiting watch world for manifest details")

	// previous emitted value, if any
	var emittedValue *manifest.FetchManifestValue

	watchLoop := world_control.NewWatchLoop(r.c.le, "", world_control.NewWaitForStateHandler(func(
		ctx context.Context,
		ws world.WorldState,
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error) {
		// Skip marking as not-idle as it doesn't help and causes unnecessary churn.
		// handler.MarkIdle(false)

		// collect manifests for the manifest ID and the desired platform IDs.
		// note that GetPlatformIds may be empty which is OK (collects for all platforms).
		var manifests []*bldr_manifest_world.CollectedManifest
		var manifestErrs []error
		var err error
		// empty means match any platform
		manifests, manifestErrs, err = bldr_manifest_world.CollectManifestsForManifestID(
			ctx,
			ws,
			r.dir.GetManifestId(),
			r.dir.GetPlatformIds(),
			r.c.conf.GetObjectKeys()...,
		)
		if err != nil {
			return true, err
		}
		for _, err := range manifestErrs {
			r.c.le.WithError(err).Warn("ignoring invalid manifest")
		}

		// filter by build types if specified
		manifests = bldr_manifest_world.FilterCollectedManifestsByBuildTypes(manifests, r.dir.GetBuildTypes())

		// filter by minimum revision if specified
		manifests = bldr_manifest_world.FilterCollectedManifestsByMinRev(manifests, r.dir.GetRev())

		// filter to latest revision for each ManifestID+PlatformID combination.
		// this sorts the slice as well.
		manifests = bldr_manifest_world.FilterCollectedManifestsByLatestRev(manifests)

		// transform to a list of ManifestRef
		manifestRefs := make([]*manifest.ManifestRef, len(manifests))
		for i, m := range manifests {
			manifestRefs[i] = &manifest.ManifestRef{
				Meta:        m.Manifest.Meta,
				ManifestRef: m.ManifestRef,
			}
		}

		// compare against the previous emitted value (if any)
		nextValue := &manifest.FetchManifestValue{ManifestRefs: manifestRefs}
		if emittedValue == nil || !nextValue.EqualVT(emittedValue) {
			// emit the next value
			emittedValue = nextValue
			if emittedValue != nil {
				_ = handler.ClearValues()
			}
			_, _ = handler.AddValue(nextValue)
			le.Debugf("fetched %v manifest(s) from world", len(manifests))
		}

		// we are done
		handler.MarkIdle(true)

		// if DisableWatch is true exit the resolver.
		if r.c.conf.GetDisableWatch() {
			return false, nil
		}

		// otherwise wait for changes.
		return true, nil
	}))

	// execute the watch loop
	return world_control.ExecuteBusWatchLoop(
		ctx,
		r.c.bus,
		r.c.conf.GetEngineId(),
		false,
		watchLoop,
	)
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
