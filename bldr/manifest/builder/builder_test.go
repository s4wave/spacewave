package bldr_manifest_builder

import (
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
)

func TestInputManifestWithDeps(t *testing.T) {
	im := NewInputManifest([]string{"src/main.ts", "src/util.ts"}, nil)
	im.ManifestDeps = []*InputManifest_ManifestDep{
		{
			ManifestId:  "spacewave-web",
			ManifestRef: &bucket.ObjectRef{BucketId: "test-bucket"},
		},
	}

	if len(im.GetFiles()) != 2 {
		t.Fatalf("expected 2 files, got %d", len(im.GetFiles()))
	}
	if len(im.GetManifestDeps()) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(im.GetManifestDeps()))
	}

	dep := im.GetManifestDeps()[0]
	if dep.GetManifestId() != "spacewave-web" {
		t.Fatalf("expected manifest id spacewave-web, got %s", dep.GetManifestId())
	}
	if dep.GetManifestRef().GetBucketId() != "test-bucket" {
		t.Fatalf("expected bucket id test-bucket, got %s", dep.GetManifestRef().GetBucketId())
	}

	// Round-trip via proto marshal/unmarshal.
	data, err := im.MarshalVT()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	im2 := &InputManifest{}
	if err := im2.UnmarshalVT(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(im2.GetManifestDeps()) != 1 {
		t.Fatalf("round-trip: expected 1 dep, got %d", len(im2.GetManifestDeps()))
	}
	dep2 := im2.GetManifestDeps()[0]
	if dep2.GetManifestId() != "spacewave-web" {
		t.Fatalf("round-trip: expected manifest id spacewave-web, got %s", dep2.GetManifestId())
	}
	if dep2.GetManifestRef().GetBucketId() != "test-bucket" {
		t.Fatalf("round-trip: expected bucket id test-bucket, got %s", dep2.GetManifestRef().GetBucketId())
	}
}
