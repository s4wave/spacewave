package manifest_fetch_world

import (
	"context"

	manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
)

// fetchManifestWatchResolver resolves FetchManifest with the controller watching for changes.
type fetchManifestWatchResolver struct {
	// c is the controller
	c *Controller
	// manifestMeta is the manifest metadata
	manifestMeta *manifest.ManifestMeta
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestWatchResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	_ = handler.ClearValues()

	// emit unique manifests keyed by manifest key
	uniqueResolver := directive.NewUniqueListXfrmResolver(
		func(v *bldr_manifest_world.CollectedManifest) string {
			return v.ManifestKey
		},
		func(k string, a, b *bldr_manifest_world.CollectedManifest) bool {
			return a.Manifest.EqualVT(b.Manifest)
		},
		func(k string, v *bldr_manifest_world.CollectedManifest) (*manifest.FetchManifestValue, bool) {
			// override the revision if necessary
			manifestMeta, manifestRef := v.Manifest.GetMeta().CloneVT(), v.ManifestRef.CloneVT()
			if overrideManifestRev := r.c.conf.GetOverrideManifestRev(); overrideManifestRev != 0 {
				manifestMeta.Rev = overrideManifestRev
			}
			return manifest.NewFetchManifestValue(manifest.NewManifestRef(manifestMeta, manifestRef)), true
		},
		handler,
	)

	// Watch the world state and re-check the manifests list when it changes.
	le := r.manifestMeta.Logger(r.c.le).WithField("engine-id", r.c.conf.GetEngineId())
	le.Debug("starting watch world for manifest details")
	defer le.Debug("exiting watch world for manifest details")

	watchLoop := world_control.NewWatchLoop(r.c.le, "", world_control.NewWaitForStateHandler(func(
		ctx context.Context,
		ws world.WorldState,
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error) {
		handler.MarkIdle(false)

		manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
			ctx,
			ws,
			r.manifestMeta.GetManifestId(),
			r.manifestMeta.GetPlatformId(),
			r.c.conf.GetObjectKeys()...,
		)
		if err != nil {
			return true, err
		}

		for _, err := range manifestErrs {
			r.c.le.WithError(err).Warn("ignoring invalid manifest")
		}

		manifestStrs := make([]string, len(manifests))
		for i := range manifestStrs {
			manifestStrs[i] = manifests[i].ManifestRef.MarshalB58()
		}
		le.Debugf("fetched %v manifest(s) from world: %v", len(manifests), manifestStrs)

		uniqueResolver.SetValues(manifests...)
		handler.MarkIdle(true)
		return true, nil
	}))

	// Execute the watch loop
	return world_control.ExecuteBusWatchLoop(
		ctx,
		r.c.bus,
		r.c.conf.GetEngineId(),
		false,
		watchLoop,
	)
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestWatchResolver)(nil))
