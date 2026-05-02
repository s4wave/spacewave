package bldr_manifest_pack

import (
	"strings"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/controllerbus/config"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/util/blockenc"
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

func TestManifestPackMetadataValidateAcceptsInlineCompressionTransformConfig(t *testing.T) {
	meta := testManifestPackMetadata(t)
	conf, err := block_transform.NewConfig([]config.Config{&transform_s2.Config{}})
	if err != nil {
		t.Fatal(err)
	}
	meta.ManifestBundleRef.TransformConf = conf
	if err := meta.Validate(); err != nil {
		t.Fatalf("Validate rejected compression transform config: %v", err)
	}
}

func TestManifestPackMetadataValidateRejectsBlockEncTransformConfig(t *testing.T) {
	meta := testManifestPackMetadata(t)
	conf, err := block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      make([]byte, 32),
	}})
	if err != nil {
		t.Fatal(err)
	}
	meta.ManifestBundleRef.TransformConf = conf
	err = meta.Validate()
	if err == nil {
		t.Fatal("Validate accepted block encryption transform config")
	}
	if !strings.Contains(err.Error(), "block encryption") {
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

func TestNewMetadataPreservesSafeManifestBundleTransformConfig(t *testing.T) {
	meta := testManifestPackMetadata(t)
	ref := meta.GetManifestBundleRef().Clone()
	conf, err := block_transform.NewConfig([]config.Config{&transform_s2.Config{}})
	if err != nil {
		t.Fatal(err)
	}
	ref.TransformConf = conf
	clean, err := NewMetadata(
		meta.GetGitSha(),
		meta.GetBuildType(),
		meta.GetProducerTarget(),
		meta.GetReactDev(),
		meta.GetCacheSchema(),
		meta.GetManifests(),
		ref,
		meta.GetPack(),
		meta.GetPackSha256(),
	)
	if err != nil {
		t.Fatalf("NewMetadata with transformed local ref = %v", err)
	}
	if clean.GetManifestBundleRef().GetTransformConf().GetEmpty() {
		t.Fatal("NewMetadata stripped safe transform config")
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
