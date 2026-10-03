package bucket

import (
	"regexp"
	"testing"
)

// TestCheckApplyBucketConfigMatchesVolume checks volume ID list and alias
// matching.
func TestCheckApplyBucketConfigMatchesVolume(t *testing.T) {
	// Describe each request and whether volume "vol-a" with alias "alias-a"
	// should answer it.
	alias := []string{"alias-a"}
	cases := []struct {
		name string
		dir  ApplyBucketConfig
		want bool
	}{
		{"any volume", NewApplyBucketConfig(nil, nil, nil), true},
		{"listed id", NewApplyBucketConfigToVolumes(nil, []string{"vol-b", "vol-a"}), true},
		{"listed alias", NewApplyBucketConfigToVolumes(nil, []string{"alias-a", "vol-b"}), true},
		{"other ids", NewApplyBucketConfigToVolumes(nil, []string{"vol-b"}), false},
		{"pattern alias", NewApplyBucketConfig(nil, regexp.MustCompile("^alias-"), nil), true},
		{"other pattern", NewApplyBucketConfig(nil, regexp.MustCompile("^vol-b$"), nil), false},
	}

	// Check each request against the volume.
	for _, tc := range cases {
		if got := CheckApplyBucketConfigMatchesVolume(tc.dir, "vol-a", alias); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
