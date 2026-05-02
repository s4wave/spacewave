package bldr_manifest_pack

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// ResolveManifestTuple resolves one manifest tuple through FetchManifest.
func ResolveManifestTuple(
	ctx context.Context,
	b bus.Bus,
	tuple *ManifestTuple,
	buildType string,
) (*bldr_manifest.ManifestRef, error) {
	if err := tuple.ValidateRequest(); err != nil {
		return nil, err
	}
	if buildType == "" {
		return nil, errors.New("build_type is empty")
	}
	dir := bldr_manifest.NewFetchManifest(
		tuple.GetManifestId(),
		[]bldr_manifest.BuildType{bldr_manifest.BuildType(buildType)},
		[]string{tuple.GetPlatformId()},
		tuple.GetRev(),
	)
	val, _, ref, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
		ctx,
		b,
		dir,
		func(isIdle bool, errs []error) (bool, error) {
			if len(errs) != 0 {
				return false, errs[0]
			}
			if isIdle {
				return false, errors.New("FetchManifest became idle without a manifest value")
			}
			return true, nil
		},
		nil,
		nil,
	)
	if ref != nil {
		defer ref.Release()
	}
	if err != nil {
		return nil, err
	}
	refs := val.GetManifestRefs()
	if len(refs) != 1 {
		return nil, errors.Errorf("FetchManifest returned %d manifest refs, want 1", len(refs))
	}
	manifestRef := refs[0].CloneVT()
	if err := validateManifestRefMatchesTuple(manifestRef, tuple, buildType); err != nil {
		return nil, err
	}
	return manifestRef, nil
}

func validateManifestRefMatchesTuple(ref *bldr_manifest.ManifestRef, tuple *ManifestTuple, buildType string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	meta := ref.GetMeta()
	if meta.GetManifestId() != tuple.GetManifestId() {
		return errors.Errorf("manifest_id mismatch: %s", meta.GetManifestId())
	}
	if meta.GetBuildType() != buildType {
		return errors.Errorf("build_type mismatch: %s", meta.GetBuildType())
	}
	if meta.GetPlatformId() != tuple.GetPlatformId() {
		return errors.Errorf("platform_id mismatch: %s", meta.GetPlatformId())
	}
	if tuple.GetRev() != 0 && meta.GetRev() != tuple.GetRev() {
		return errors.Errorf("rev mismatch: %d", meta.GetRev())
	}
	return nil
}
