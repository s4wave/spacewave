//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceFilePreservesInstalledBinary covers failed and successful updates.
func TestReplaceFilePreservesInstalledBinary(t *testing.T) {
	// Keep a runnable installation when the staged download is missing.
	dir := t.TempDir()
	installed := filepath.Join(dir, "app.exe")
	staged := filepath.Join(dir, "app.exe.copying")
	if err := os.WriteFile(installed, []byte("version A"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(staged, installed); err == nil {
		t.Fatal("replacement accepted a missing staged file")
	}
	dat, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if string(dat) != "version A" {
		t.Fatalf("failed replacement changed installed bytes: %q", dat)
	}

	// Replace the existing file once the staged download is complete.
	if err := os.WriteFile(staged, []byte("version B"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(staged, installed); err != nil {
		t.Fatal(err)
	}
	dat, err = os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if string(dat) != "version B" {
		t.Fatalf("replacement did not install staged bytes: %q", dat)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged file remains after replacement: %v", err)
	}
}
