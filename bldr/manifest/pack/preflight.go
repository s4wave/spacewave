package bldr_manifest_pack

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/world"
)

// VerifyImportedManifests verifies every metadata tuple is collectable.
func VerifyImportedManifests(
	ctx context.Context,
	ws world.WorldState,
	meta *ManifestPackMetadata,
) error {
	if err := meta.Validate(); err != nil {
		return err
	}
	bundle, err := readManifestBundle(ctx, ws, meta.GetManifestBundleRef())
	if err != nil {
		return err
	}
	if len(bundle.GetManifestRefs()) != len(meta.GetManifests()) {
		return errors.Errorf("manifest bundle count mismatch: got %d want %d", len(bundle.GetManifestRefs()), len(meta.GetManifests()))
	}
	for i, tuple := range meta.GetManifests() {
		if err := validateManifestRefMatchesTuple(bundle.GetManifestRefs()[i], tuple, meta.GetBuildType()); err != nil {
			return errors.Wrapf(err, "manifest %d", i)
		}
		roots := append([]string{tuple.GetObjectKey()}, tuple.GetLinkObjectKeys()...)
		for _, root := range roots {
			if err := verifyImportedManifestRoot(ctx, ws, tuple, root, bundle.GetManifestRefs()[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyImportedManifestRoot(
	ctx context.Context,
	ws world.WorldState,
	tuple *ManifestTuple,
	root string,
	expected *bldr_manifest.ManifestRef,
) error {
	collected, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		tuple.GetManifestId(),
		[]string{tuple.GetPlatformId()},
		root,
	)
	if err != nil {
		return errors.Wrapf(err, "collect manifests from %s", root)
	}
	if len(manifestErrs) != 0 {
		return errors.Errorf("collect manifests from %s had skipped manifests: %s", root, joinErrors(manifestErrs))
	}
	if len(collected) != 1 {
		return errors.Errorf("manifest tuple %s@%s not found from %s", tuple.GetManifestId(), tuple.GetPlatformId(), root)
	}
	got := collected[0]
	if err := validateManifestRefMatchesTuple(bldr_manifest.NewManifestRef(got.Manifest.GetMeta(), got.ManifestRef), tuple, expected.GetMeta().GetBuildType()); err != nil {
		return errors.Wrapf(err, "manifest tuple %s@%s from %s", tuple.GetManifestId(), tuple.GetPlatformId(), root)
	}
	if !got.ManifestRef.EqualVT(expected.GetManifestRef()) {
		return errors.Errorf("manifest tuple %s@%s from %s has unexpected root ref", tuple.GetManifestId(), tuple.GetPlatformId(), root)
	}
	return nil
}

func joinErrors(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}
