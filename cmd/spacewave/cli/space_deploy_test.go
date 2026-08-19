//go:build !js

package spacewave_cli

import (
	"strings"
	"testing"
)

func TestSpaceDeployCommandAdvertisesManifestSetDefault(t *testing.T) {
	statePath := ""
	sessionIdx := uint(1)
	cmd := newSpaceDeployCommand(&statePath, &sessionIdx)
	if !strings.Contains(cmd.Usage, "manifest set") {
		t.Fatalf("usage = %q", cmd.Usage)
	}
	found := false
	for _, flag := range cmd.Flags {
		if strings.Contains(flag.String(), "plugin-host") {
			found = true
		}
	}
	if !found {
		t.Fatal("object-key flag does not advertise plugin-host default")
	}
}
