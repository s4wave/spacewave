package bldr_manifest_builder

import (
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
