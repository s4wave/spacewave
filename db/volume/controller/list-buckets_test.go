package volume_controller

import (
	"testing"

	"github.com/s4wave/spacewave/db/volume"
)

// TestCheckListBucketsMatchesVolume checks volume ID list and alias matching.
func TestCheckListBucketsMatchesVolume(t *testing.T) {
	// Describe each request and whether volume "vol-a" with alias "alias-a"
	// should answer it.
	alias := []string{"alias-a"}
	cases := []struct {
		name string
		dir  volume.ListBuckets
		want bool
	}{
		{"any volume", volume.NewListBuckets("", nil), true},
		{"listed id", volume.NewListBuckets("", []string{"vol-b", "vol-a"}), true},
		{"listed alias", volume.NewListBuckets("", []string{"alias-a", "vol-b"}), true},
		{"other ids", volume.NewListBuckets("", []string{"vol-b"}), false},
		{"pattern id", volume.NewListBucketsWithRe("", "^vol-a$"), true},
		{"pattern alias", volume.NewListBucketsWithRe("", "^alias-"), true},
		{"other pattern", volume.NewListBucketsWithRe("", "^vol-b$"), false},
	}

	// Check each request against the volume.
	for _, tc := range cases {
		if got := checkListBucketsMatchesVolume(tc.dir, "vol-a", alias); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
