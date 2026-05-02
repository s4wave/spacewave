package bldr_manifest_pack

import (
	"strings"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestManifestPackMetadataValidateAcceptsCleanMetadata(t *testing.T) {
	meta := testManifestPackMetadata(t)
	if err := meta.Validate(); err != nil {
		t.Fatalf("Validate clean metadata = %v", err)
	}
}

func TestManifestPackMetadataValidateAcceptsZeroManifestRev(t *testing.T) {
	meta := testManifestPackMetadata(t)
	meta.Manifests[0].Rev = 0
	if err := meta.Validate(); err != nil {
		t.Fatalf("Validate metadata with zero manifest rev = %v", err)
	}
}

func TestManifestPackMetadataValidateRejectsWrongPackDigestLength(t *testing.T) {
	meta := testManifestPackMetadata(t)
	meta.PackSha256 = []byte("short")
	err := meta.Validate()
	if err == nil {
		t.Fatal("Validate accepted short pack_sha256")
	}
	if !strings.Contains(err.Error(), "pack_sha256") {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestManifestPackMetadataValidateRejectsInlineTransformConfig(t *testing.T) {
	meta := testManifestPackMetadata(t)
	meta.ManifestBundleRef.TransformConf = &block_transform.Config{
		Steps: []*block_transform.StepConfig{{
			Id:     "blockenc",
			Config: []byte("secret"),
		}},
	}
	err := meta.Validate()
	if err == nil {
		t.Fatal("Validate accepted inline transform config")
	}
	if !strings.Contains(err.Error(), "inline transform config") {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestManifestPackMetadataValidateRejectsTransformConfigRef(t *testing.T) {
	meta := testManifestPackMetadata(t)
	ref, err := block.BuildBlockRef([]byte("transform config"), nil)
	if err != nil {
		t.Fatal(err)
	}
	meta.ManifestBundleRef.TransformConfRef = ref
	err = meta.Validate()
	if err == nil {
		t.Fatal("Validate accepted transform config ref")
	}
	if !strings.Contains(err.Error(), "transform config ref") {
		t.Fatalf("Validate error = %v", err)
	}
}

func testManifestPackMetadata(t *testing.T) *ManifestPackMetadata {
	t.Helper()

	rootRef, err := block.BuildBlockRef([]byte("manifest bundle"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &ManifestPackMetadata{
		FormatVersion:  MetadataFormatVersion,
		GitSha:         "0123456789abcdef0123456789abcdef01234567",
		BuildType:      "release",
		ProducerTarget: "release-remote-js",
		Manifests: []*ManifestTuple{{
			ManifestId:     "spacewave-web",
			PlatformId:     "js",
			Rev:            7,
			ObjectKey:      "ci/manifest-pack/spacewave-web/js",
			LinkObjectKeys: []string{"ci/manifest-pack"},
		}},
		ManifestBundleRef: &bucket.ObjectRef{
			RootRef: rootRef,
		},
		Pack: &packfile.PackfileEntry{
			Id:                 "pfv1_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			BloomFilter:        []byte{1, 2, 3},
			BlockCount:         3,
			SizeBytes:          1024,
			CreatedAt:          timestamppb.Now(),
			BloomFormatVersion: packfile.BloomFormatVersionV1,
		},
		PackSha256: []byte{
			0, 1, 2, 3, 4, 5, 6, 7,
			8, 9, 10, 11, 12, 13, 14, 15,
			16, 17, 18, 19, 20, 21, 22, 23,
			24, 25, 26, 27, 28, 29, 30, 31,
		},
		CacheSchema: "manifest-pack-v1",
	}
}
