package gocompiler

import (
	"slices"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

func TestNewBuildTagsDoNotDependOnReleaseEnv(t *testing.T) {
	for _, env := range []string{"", "prod", "staging"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("SPACEWAVE_RELEASE_ENV", env)
			tags := NewBuildTags(bldr_manifest.BuildType_RELEASE, false)
			if !slices.Equal(tags, []string{"build_type_release", "purego"}) {
				t.Fatalf("build tags = %v, want release defaults only", tags)
			}
		})
	}
}
