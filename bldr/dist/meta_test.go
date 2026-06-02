package bldr_dist

import (
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
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

func TestDistMetaValidateEntrypointRole(t *testing.T) {
	input := NewDistEntrypointMeta(
		"project",
		"desktop/darwin/arm64",
		[]string{"test-plugin"},
		&bucket.ObjectRef{},
		"dist",
		EntrypointRoleCLI,
		"stable",
		"spacewave-dist",
		42,
	)
	if err := input.Validate(); err != nil {
		t.Fatal(err.Error())
	}

	input.EntrypointRole = "plugin"
	if err := input.Validate(); err == nil || !strings.Contains(err.Error(), "entrypoint_role") {
		t.Fatalf("expected entrypoint role error, got %v", err)
	}
}
