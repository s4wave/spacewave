package manifest_fetch_world

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/sirupsen/logrus"
)

// fetchManifestResolver resolves FetchManifest with the controller optionally watching for changes.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// dir is the FetchManifest directive
	dir manifest.FetchManifest
	// emittedValue is the previously emitted value, if any.
	emittedValue *manifest.FetchManifestValue
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	_ = handler.ClearValues()

	// Watch the world state and re-check the manifests list when it changes.
	le := r.c.le.WithField("engine-id", r.c.conf.GetEngineId()).WithField("manifest-id", r.dir.GetManifestId())
	le.Debug("starting watch world for manifest details")
	defer le.Debug("exiting watch world for manifest details")

	watchLoop := world_control.NewWatchLoop(r.c.le, "", world_control.NewWaitForStateHandler(func(
		ctx context.Context,
		ws world.WorldState,
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error) {
		return r.reconcileManifests(ctx, le, handler, ws)
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

// reconcileManifestsCore re-collects the manifests for this directive from the
// given world state and emits them when they differ from the last emission. It
// returns whether the watch loop should wait for further changes.
func (r *fetchManifestResolver) reconcileManifestsCore(
	ctx context.Context,
	le *logrus.Entry,
	handler directive.ResolverHandler,
	ws world.WorldState,
) (bool, error) {
	// Skip marking as not-idle as it doesn't help and causes unnecessary churn.
	// handler.MarkIdle(false)

	// Collect the graph once for all resolver IDs and copy this ID's slice before
	// the filtering helpers remove or reorder entries.
	snapshot, err := r.c.collectManifests(ctx, ws)
	if err != nil {
		return true, err
	}
	manifests := slices.Clone(snapshot.manifests[r.dir.GetManifestId()])
	for _, manifestErr := range snapshot.manifestErrs {
		r.c.le.WithError(manifestErr).Warn("ignoring invalid manifest")
	}

	// Filter by platform IDs when the directive restricts them.
	if platformIDs := r.dir.GetPlatformIds(); len(platformIDs) != 0 {
		manifests = bldr_manifest_world.FilterCollectedManifestsByPlatformID(manifests, platformIDs)
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

	// A cache miss is absence of a value, not a successful zero-ref
	// FetchManifestValue. Devtool builder resolvers can share the same
	// directive, and early empty cache values can otherwise win startup
	// races before builders publish their manifest refs.
	if len(manifestRefs) == 0 {
		if r.emittedValue != nil {
			r.emittedValue = nil
			_ = handler.ClearValues()
		}
		le.Debugf("fetched %v manifest(s) from world", len(manifests))
	} else {
		nextValue := &manifest.FetchManifestValue{ManifestRefs: manifestRefs}
		if r.emittedValue == nil || !nextValue.EqualVT(r.emittedValue) {
			r.emittedValue = nextValue
			_ = handler.ClearValues()
			_, _ = handler.AddValue(nextValue)
			le.Debugf("fetched %v manifest(s) from world", len(manifests))
		}
	}

	// we are done
	handler.MarkIdle(true)

	// if DisableWatch is true exit the resolver.
	if r.c.conf.GetDisableWatch() {
		return false, nil
	}

	// otherwise wait for changes.
	return true, nil
}

// _ is a type assertion
var _ directive.Resolver = (*fetchManifestResolver)(nil)
