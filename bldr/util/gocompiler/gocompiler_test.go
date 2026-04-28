package gocompiler

import (
	"slices"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

func TestNewBuildTagsProductionRelease(t *testing.T) {
	t.Setenv("SPACEWAVE_RELEASE_ENV", "prod")
	tags := NewBuildTags(bldr_manifest.BuildType_RELEASE, false)
	if !slices.Contains(tags, "prod_signing") {
		t.Fatalf("expected prod_signing tag in %v", tags)
	}
}

func TestNewBuildTagsStagingRelease(t *testing.T) {
	t.Setenv("SPACEWAVE_RELEASE_ENV", "staging")
	tags := NewBuildTags(bldr_manifest.BuildType_RELEASE, false)
	if slices.Contains(tags, "prod_signing") {
		t.Fatalf("did not expect prod_signing tag in %v", tags)
	}
}
