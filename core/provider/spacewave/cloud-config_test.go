package provider_spacewave

import "testing"

func TestCloudAuthConfigURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "staging serving origin",
			endpoint: "/",
			want:     "/api/auth/config",
		},
		{
			name:     "production serving origin",
			endpoint: "/",
			want:     "/api/auth/config",
		},
		{
			name:     "local dev production fallback",
			endpoint: "https://spacewave.app",
			want:     "https://spacewave.app/api/auth/config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cloudAuthConfigURL(tt.endpoint)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("URL = %q, want %q", got, tt.want)
			}
		})
	}
}
