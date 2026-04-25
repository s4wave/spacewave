//go:build !js

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCodesignVerifyGate: on darwin, an unsigned tree must be rejected so it
// never advertises STAGED. On other platforms the stub is a no-op, which we
// assert explicitly so nobody silently breaks the non-darwin build.
func TestCodesignVerifyGate(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "Unsigned.app")
	if err := os.MkdirAll(filepath.Join(fake, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatalf("seed fake .app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fake, "Contents", "Info.plist"), []byte("<plist></plist>"), 0o644); err != nil {
		t.Fatalf("seed Info.plist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fake, "Contents", "MacOS", "spacewave"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("seed binary: %v", err)
	}

	err := verifyAppBundleCodesign(t.Context(), fake)
	if runtime.GOOS == "darwin" {
		if err == nil {
			t.Fatal("expected codesign verify to fail on unsigned .app, got nil")
		}
		return
	}
	if err != nil {
		t.Fatalf("non-darwin stub should pass, got %v", err)
	}
}
