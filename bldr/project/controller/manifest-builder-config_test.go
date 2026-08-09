//go:build !js

package bldr_project_controller

import (
	"strings"
	"testing"
)

func TestManifestBuilderConfigRejectsInvalidPlatform(t *testing.T) {
	config := NewManifestBuilderConfig("app", "dev", "invalid-platform", "origin")
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "platform_id") {
		t.Fatalf("expected platform error, got %v", err)
	}
}
