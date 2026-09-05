package provider_local

import "testing"

// TestConfigSignalingURL covers the browser's same-origin signaling configuration
// and the absolute endpoints used by native runtimes.
func TestConfigSignalingURL(t *testing.T) {
	for _, raw := range []string{"", "/", "/cloud", "https://spacewave.app", "http://localhost:8080"} {
		t.Run(raw, func(t *testing.T) {
			conf := &Config{SignalingUrl: raw}
			if err := conf.Validate(); err != nil {
				t.Fatalf("valid signaling URL %q rejected: %v", raw, err)
			}
		})
	}
	for _, raw := range []string{"cloud", "//other.example", "///other.example", "https:/cloud", "ftp://cloud.example", "/%zz"} {
		t.Run(raw, func(t *testing.T) {
			conf := &Config{SignalingUrl: raw}
			if err := conf.Validate(); err == nil {
				t.Fatalf("invalid signaling URL %q accepted", raw)
			}
		})
	}
}
