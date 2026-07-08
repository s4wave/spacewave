package cdn_test

import (
	"os"
	"testing"

	"github.com/s4wave/spacewave/core/cdn"
)

func TestBaseURLUsesDefaultWhenEnvironmentOverrideUnset(t *testing.T) {
	unsetEnv(t, "SPACEWAVE_CDN_BASE_URL")

	if got := cdn.BaseURL(); got != cdn.DefaultBaseURL {
		t.Fatalf("BaseURL() with no SPACEWAVE_CDN_BASE_URL = %q, want %q", got, cdn.DefaultBaseURL)
	}
}

func TestBaseURLHonorsEnvironmentOverride(t *testing.T) {
	const override = "https://staging-cdn.example.test"
	t.Setenv("SPACEWAVE_CDN_BASE_URL", override)

	if got := cdn.BaseURL(); got != override {
		t.Fatalf("BaseURL() with SPACEWAVE_CDN_BASE_URL set = %q, want %q", got, override)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	oldValue, hadValue := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		var err error
		if hadValue {
			err = os.Setenv(key, oldValue)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Fatalf("restore %s: %v", key, err)
		}
	})
}
