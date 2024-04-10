package bldr_dist

import (
	"testing"

	"github.com/aperturerobotics/hydra/bucket"
)

func TestDistMetaB58(t *testing.T) {
	input := &DistMeta{
		ProjectId:  "project",
		PlatformId: "dist-platform",
	}
	inputB58 := input.MarshalB58()
	output, err := UnmarshalDistMetaB58(inputB58)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !output.EqualVT(input) {
		t.Fail()
	}
}

func TestDistMetaValidate(t *testing.T) {
	// mostly checking to make sure the dist entrypoint doesn't fail with a reasonable meta
	input := &DistMeta{
		ProjectId:      "project",
		PlatformId:     "dist-platform",
		StartupPlugins: []string{"test-plugin"},
		DistWorldRef:   &bucket.ObjectRef{},
		DistObjectKey:  "dist",
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err.Error())
	}
}
