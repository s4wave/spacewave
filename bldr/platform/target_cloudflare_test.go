package bldr_platform

import (
	"slices"
	"testing"
)

func TestCloudflareWorkersTarget(t *testing.T) {
	target, err := ParseTarget(TargetID_CloudflareWorkers)
	if err != nil {
		t.Fatal(err)
	}
	if got := target.GetPlatformIDs(); !slices.Equal(got, []string{PlatformID_CLOUDFLARE}) {
		t.Fatalf("platform IDs = %v, want [%s]", got, PlatformID_CLOUDFLARE)
	}
	if !slices.Contains(ListBuiltinTargetIDs(), TargetID_CloudflareWorkers) {
		t.Fatalf("builtin targets omit %q", TargetID_CloudflareWorkers)
	}
}
