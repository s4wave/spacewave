package manifest_fetch_world

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/sirupsen/logrus"
)

// fetchManifestResolver resolves FetchManifest with the controller optionally watching for changes.
type fetchManifestResolver struct {
	// c supplies the World and release authority configuration.
	c *Controller
	// dir selects the requested Manifest metadata.
	dir manifest.FetchManifest
	// emittedValue is the previously emitted value, if any.
	emittedValue *manifest.FetchManifestValue
	// releasePeers holds the launcher's resolved pins for this RPC lifetime.
	releasePeers []peer.ID
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Local and embedded Worlds use their existing resolution path.
	if r.c.conf.GetReleaseAuthorityPluginId() == "" {
		return r.resolveWorld(ctx, handler)
	}

	// Hold the embedded launcher's client while resolving executable manifests.
	return plugin.ExPluginLoadAccessClient(ctx, r.c.bus, r.c.conf.GetReleaseAuthorityPluginId(), func(ctx context.Context, client srpc.Client) error {
		// Inherit the launcher's resolved pins instead of configuring another key.
		response, err := manifest.NewSRPCReleaseAuthorityClient(client).GetReleasePeerIds(ctx, &manifest.GetReleasePeerIdsRequest{})
		if err != nil {
			return errors.Wrap(err, "release authorization: resolve launcher pins")
		}
		peers, err := confparse.ParsePeerIDs(response.GetPeerIds(), false)
		if err != nil {
			return errors.Wrap(err, "release authorization: launcher pins")
		}
		if len(peers) == 0 {
			return errors.New("release authorization: missing launcher release peer pins")
		}
		r.releasePeers = peers
		return r.resolveWorld(ctx, handler)
	})
}

// resolveWorld watches the mounted World for authorized executable selections.
func (r *fetchManifestResolver) resolveWorld(ctx context.Context, handler directive.ResolverHandler) error {
	// Register the resolver and clear any previously emitted values.
	r.c.addResolver(r)
	defer r.c.removeResolver(r)
	r.emittedValue = nil
	_ = handler.ClearValues()

	// Watch the world state and re-check the manifests list when it changes.
	le := r.c.le.WithField("engine-id", r.c.conf.GetEngineId()).WithField("manifest-id", r.dir.GetManifestId())
	le.Debug("starting watch world for manifest details")
	defer le.Debug("exiting watch world for manifest details")

	// Build the watch loop that reconciles manifests on each state change.
	watchLoop := world_control.NewWatchLoop(r.c.le, "", world_control.NewWaitForStateHandler(func(
		ctx context.Context,
		ws world.WorldState,
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error) {
		// Authorization failures withdraw old values and terminate with the rejection.
		wait, err := r.reconcileManifests(ctx, le, handler, ws)
		if err != nil && r.c.conf.GetReleaseAuthorityPluginId() != "" {
			r.emittedValue = nil
			_ = handler.ClearValues()
			return false, err
		}
		return wait, err
	}))

	// Report the directive idle while the World engine is unavailable, so a
	// caller waiting for idle values does not block on an unreachable World.
	// The lookup can go idle holding the engine, which ExecWaitValue offers to
	// the value filter before the idle callback, under one lock.
	var found bool
	_, _, engineRef, err := bus.ExecWaitValue(
		ctx,
		r.c.bus,
		world.NewLookupWorldEngine(r.c.conf.GetEngineId()),
		func(isIdle bool, _ []error) (bool, error) {
			if isIdle && !found {
				handler.MarkIdle(true)
			}
			return true, nil
		},
		nil,
		func(world.LookupWorldEngineValue) (bool, error) {
			found = true
			return true, nil
		},
	)
	if err != nil {
		return err
	}
	defer engineRef.Release()

	// Resolve the mounted World's snapshots for this directive lifetime.
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
	// A private slice lets this directive narrow the shared collection independently.
	snapshot, err := r.c.collectManifests(ctx, ws)
	if err != nil {
		return true, err
	}
	manifests := slices.Clone(snapshot.manifests[r.dir.GetManifestId()])
	for _, manifestErr := range snapshot.manifestErrs {
		r.c.le.WithError(manifestErr).Warn("ignoring invalid manifest")
	}

	// Directive constraints select the latest eligible manifest for each platform.
	if platformIDs := r.dir.GetPlatformIds(); len(platformIDs) != 0 {
		manifests = bldr_manifest_world.FilterCollectedManifestsByPlatformID(manifests, platformIDs)
	}
	manifests = bldr_manifest_world.FilterCollectedManifestsByBuildTypes(manifests, r.dir.GetBuildTypes())
	manifests = bldr_manifest_world.FilterCollectedManifestsByMinRev(manifests, r.dir.GetRev())
	manifests = bldr_manifest_world.FilterCollectedManifestsByLatestRev(manifests)

	// Resolver values expose immutable references rather than collection records.
	manifestRefs := make([]*manifest.ManifestRef, len(manifests))
	for i, m := range manifests {
		manifestRefs[i] = &manifest.ManifestRef{
			Meta:        m.Manifest.Meta,
			ManifestRef: m.ManifestRef,
		}
		if r.c.conf.GetReleaseAuthorityPluginId() != "" {
			// Direct Manifest objects have no independent authorization envelope.
			objType, err := world_types.GetObjectType(ctx, ws, m.ManifestKey)
			if err != nil {
				return true, errors.Wrap(err, "release authorization: read manifest type")
			}
			if objType == bldr_manifest_world.ManifestTypeID {
				return true, errors.Errorf("manifest %s: release authorization: missing authorization", m.ManifestKey)
			}

			// Read authorization beside the selected root.
			candidate, _, err := bldr_manifest_world.LookupManifestRef(ctx, ws, m.ManifestKey)
			if err != nil {
				return true, errors.Wrap(err, "release authorization: read manifest reference")
			}
			manifestRefs[i].ReleaseAuthorization = candidate.GetReleaseAuthorization().CloneVT()
			if err := manifestRefs[i].VerifyReleaseAuthorization(r.releasePeers); err != nil {
				return true, errors.Wrapf(err, "manifest %s", m.ManifestKey)
			}
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
	}
	if len(manifestRefs) != 0 {
		nextValue := &manifest.FetchManifestValue{ManifestRefs: manifestRefs}
		if r.emittedValue == nil || !nextValue.EqualVT(r.emittedValue) {
			r.emittedValue = nextValue
			_ = handler.ClearValues()
			_, _ = handler.AddValue(nextValue)
			le.Debugf("fetched %v manifest(s) from world", len(manifests))
		}
	}

	// Idle marks this snapshot as settled before an optional World watch.
	handler.MarkIdle(true)
	if r.c.conf.GetDisableWatch() {
		return false, nil
	}
	return true, nil
}

// _ is a type assertion
var _ directive.Resolver = (*fetchManifestResolver)(nil)
