//go:build !js

package main

import "testing"

func TestNeedsBuilderImage(t *testing.T) {
	tests := []struct {
		name      string
		hostGOOS  string
		platforms []string
		want      bool
	}{
		{
			name:      "darwin only never needs docker builder",
			hostGOOS:  "darwin",
			platforms: []string{"darwin-amd64", "darwin-arm64"},
			want:      false,
		},
		{
			name:      "linux needs docker builder",
			hostGOOS:  "linux",
			platforms: []string{"linux-amd64"},
			want:      true,
		},
		{
			name:      "windows on linux needs docker builder",
			hostGOOS:  "linux",
			platforms: []string{"windows-amd64"},
			want:      true,
		},
		{
			name:      "windows on windows builds natively",
			hostGOOS:  "windows",
			platforms: []string{"windows-amd64", "windows-arm64"},
			want:      false,
		},
		{
			name:      "windows and linux on windows still need docker builder for linux",
			hostGOOS:  "windows",
			platforms: []string{"windows-amd64", "linux-amd64"},
			want:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := needsBuilderImage(test.hostGOOS, test.platforms)
			if got != test.want {
				t.Fatalf("needsBuilderImage(%q, %#v) = %v, want %v", test.hostGOOS, test.platforms, got, test.want)
			}
		})
	}
}
