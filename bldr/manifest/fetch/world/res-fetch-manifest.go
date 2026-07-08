package manifest_fetch_world

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	spacewave_release "github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
)

const releaseMetadataDirectoryObjectKey = "release/metadata"

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

		// Prefer ReleaseMetadata.ManifestRefs when configured. The CDN Release
		// World graph can be slower to traverse than the quickstart registration
		// gate, while metadata is the signed release index for the same refs.
		worldManifestCount := 0
		manifestRefs, err := r.collectReleaseMetadataManifestRefs(ctx, ws)
		if err != nil {
			return true, err
		}
		if len(manifestRefs) != 0 {
			le.Debugf("fetched %v manifest(s) from release metadata", len(manifestRefs))
		} else {
			var manifests []*bldr_manifest_world.CollectedManifest
			var manifestErrs []error
			// empty means match any platform
			manifests, manifestErrs, err = bldr_manifest_world.CollectManifestsForManifestIDResettingUnsupportedHash(
				ctx,
				r.c.le,
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
			worldManifestCount = len(manifests)
			manifestRefs = make([]*manifest.ManifestRef, len(manifests))
			for i, m := range manifests {
				manifestRefs[i] = &manifest.ManifestRef{
					Meta:        m.Manifest.Meta,
					ManifestRef: m.ManifestRef,
				}
			}
			le.Debugf("fetched %v manifest(s) from world", len(manifests))
		}

		// A cache miss is absence of a value, not a successful zero-ref
		// FetchManifestValue. Devtool builder resolvers can share the same
		// directive, and early empty cache values can otherwise win startup
		// races before builders publish their manifest refs.
		if len(manifestRefs) == 0 {
			if emittedValue != nil {
				emittedValue = nil
				_ = handler.ClearValues()
			}
			le.Debugf("fetched %v manifest(s) from world", worldManifestCount)
		} else {
			nextValue := &manifest.FetchManifestValue{ManifestRefs: manifestRefs}
			if emittedValue == nil || !nextValue.EqualVT(emittedValue) {
				emittedValue = nextValue
				_ = handler.ClearValues()
				_, _ = handler.AddValue(nextValue)
				le.Debugf("fetched %v manifest(s) from world", worldManifestCount)
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

func (r *fetchManifestResolver) collectReleaseMetadataManifestRefs(
	ctx context.Context,
	ws world.WorldState,
) ([]*manifest.ManifestRef, error) {
	channelKey := r.c.conf.GetReleaseMetadataChannelKey()
	if channelKey == "" {
		return nil, nil
	}
	metadata, err := readSelectedReleaseMetadata(ctx, ws, channelKey)
	if err != nil {
		return nil, errors.Wrap(err, "read release metadata")
	}
	return filterReleaseMetadataManifestRefs(
		metadata,
		r.dir.GetManifestId(),
		r.dir.GetPlatformIds(),
		r.dir.GetBuildTypes(),
		r.dir.GetRev(),
	)
}

func readSelectedReleaseMetadata(
	ctx context.Context,
	ws world.WorldState,
	channelKey string,
) (*spacewave_release.ReleaseMetadata, error) {
	directory, err := readReleaseMetadataBlock[*spacewave_release.ChannelDirectory](
		ctx,
		ws,
		releaseMetadataDirectoryObjectKey,
		func() block.Block { return &spacewave_release.ChannelDirectory{} },
	)
	if err != nil {
		return nil, errors.Wrap(err, "read release channel directory")
	}
	if err := directory.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate release channel directory")
	}
	for _, entry := range directory.GetChannels() {
		if entry.GetChannelKey() != channelKey {
			continue
		}
		if ref := entry.GetReleaseMetadataRef(); ref == nil || ref.GetEmpty() {
			return nil, errors.New("release metadata missing for channel " + channelKey)
		}
		metadata, err := readReleaseMetadataBlock[*spacewave_release.ReleaseMetadata](
			ctx,
			ws,
			releaseMetadataObjectKey(channelKey),
			func() block.Block { return &spacewave_release.ReleaseMetadata{} },
		)
		if err != nil {
			return nil, errors.Wrap(err, "read release metadata for channel "+channelKey)
		}
		if err := metadata.Validate(); err != nil {
			return nil, errors.Wrap(err, "validate release metadata")
		}
		if metadata.GetChannelKey() != channelKey {
			return nil, errors.New("release metadata channel key mismatch")
		}
		return metadata, nil
	}
	return nil, errors.New("release metadata missing for channel " + channelKey)
}

func readReleaseMetadataBlock[T block.Block](
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	ctor func() block.Block,
) (T, error) {
	var out T
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return out, err
	}
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		blk, err := block.UnmarshalBlock[block.Block](ctx, bcs, ctor)
		if err != nil {
			return err
		}
		typed, ok := blk.(T)
		if !ok {
			return errors.New("release metadata block type mismatch")
		}
		out = typed
		return nil
	})
	return out, err
}

func releaseMetadataObjectKey(channelKey string) string {
	return releaseMetadataDirectoryObjectKey + "/" + channelKey
}

func filterReleaseMetadataManifestRefs(
	metadata *spacewave_release.ReleaseMetadata,
	manifestID string,
	platformIDs []string,
	buildTypes []manifest.BuildType,
	minRev uint64,
) ([]*manifest.ManifestRef, error) {
	latest := make(map[string]*manifest.ManifestRef)
	for _, ref := range metadata.GetManifestRefs() {
		if err := ref.Validate(); err != nil {
			return nil, errors.Wrap(err, "release metadata manifest ref")
		}
		meta := ref.GetMeta()
		if manifestID != "" && meta.GetManifestId() != manifestID {
			continue
		}
		if len(platformIDs) != 0 && !slices.Contains(platformIDs, meta.GetPlatformId()) {
			continue
		}
		if len(buildTypes) != 0 && !slices.Contains(buildTypes, manifest.BuildType(meta.GetBuildType())) {
			continue
		}
		if minRev != 0 && meta.GetRev() < minRev {
			continue
		}
		key := meta.GetManifestId() + "\x00" + meta.GetPlatformId()
		if prev := latest[key]; prev == nil || meta.GetRev() > prev.GetMeta().GetRev() {
			latest[key] = ref.CloneVT()
		}
	}
	out := make([]*manifest.ManifestRef, 0, len(latest))
	for _, ref := range latest {
		out = append(out, ref)
	}
	slices.SortFunc(out, func(a, b *manifest.ManifestRef) int {
		am := a.GetMeta()
		bm := b.GetMeta()
		if cmp := strings.Compare(am.GetManifestId(), bm.GetManifestId()); cmp != 0 {
			return cmp
		}
		if am.GetRev() > bm.GetRev() {
			return -1
		}
		if am.GetRev() < bm.GetRev() {
			return 1
		}
		return strings.Compare(am.GetPlatformId(), bm.GetPlatformId())
	})
	return out, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
