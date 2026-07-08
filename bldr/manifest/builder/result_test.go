package bldr_manifest_builder

import (
	"errors"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestBuilderResultValidateManifestRefMetaMismatch(t *testing.T) {
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	result := NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		NewInputManifest([]string{"main.go"}, nil),
	)
	result.ManifestRef.Meta = bldr_manifest.NewManifestMeta(
		"demo",
		bldr_manifest.BuildType_DEV,
		"desktop/linux/arm64",
		1,
	)

	if err := result.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBuilderResultValidateBuildCacheAllowsAssetOnlyManifest(t *testing.T) {
	meta := bldr_manifest.NewManifestMeta("asset-only", bldr_manifest.BuildType_DEV, "js", 1)
	result := NewBuilderResult(
		bldr_manifest.NewManifest(meta, ""),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		NewInputManifest([]string{"dist/assets.json"}, nil),
	)

	if err := result.ValidateBuildCache(); err != nil {
		t.Fatalf("ValidateBuildCache rejected asset-only manifest: %v", err)
	}
	if err := result.Validate(); !errors.Is(err, bldr_manifest.ErrEmptyEntrypoint) {
		t.Fatalf("Validate error = %v, want ErrEmptyEntrypoint", err)
	}
}
