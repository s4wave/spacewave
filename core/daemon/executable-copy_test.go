//go:build !js

package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrepareVerifiedExecutableRejectsChangedSource prevents a staged path
// from supplying different bytes after the launcher accepts its digest.
func TestPrepareVerifiedExecutableRejectsChangedSource(t *testing.T) {
	root := daemonTestRoot(t)
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "spacewave")
	accepted := []byte("accepted executable")
	if err := os.WriteFile(source, accepted, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(accepted)
	selected := hex.EncodeToString(digest[:])
	if err := os.WriteFile(source, []byte("different executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareVerifiedExecutable(state, source, selected); err == nil || !strings.Contains(err.Error(), "fails accepted digest") {
		t.Fatalf("changed source accepted: %v", err)
	}
	if err := os.WriteFile(source, accepted, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareVerifiedExecutable(state, source, selected); err != nil {
		t.Fatalf("accepted source rejected: %v", err)
	}
}

// TestCopyExecutablePublishesVerifiedVersions exercises the desktop bootstrap
// with a bundled source and keeps an earlier digest path intact for a live daemon.
func TestCopyExecutablePublishesVerifiedVersions(t *testing.T) {
	// Place source bytes in a fake bundle and select an independent state root.
	root := daemonTestRoot(t)
	bundle := filepath.Join(root, "Spacewave.app", "Contents", "MacOS")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(bundle, "spacewave")
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("first executable"), 0o700); err != nil {
		t.Fatal(err)
	}

	// Reusing the same source must verify and return the same immutable path.
	first, err := PrepareExecutable(state, source)
	if err != nil {
		t.Fatal(err)
	}
	again, err := PrepareExecutable(state, source)
	if err != nil {
		t.Fatal(err)
	}
	if first != again || strings.Contains(first, ".app") {
		t.Fatalf("verified copy path: first=%q again=%q", first, again)
	}
	if data, err := os.ReadFile(first); err != nil || string(data) != "first executable" {
		t.Fatalf("copied executable: %q, %v", data, err)
	}

	// A newer source gets a new name while the old executable stays intact.
	if err := os.WriteFile(source, []byte("second executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	second, err := PrepareExecutable(state, source)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("changed source reused the running executable path")
	}
	if data, err := os.ReadFile(first); err != nil || string(data) != "first executable" {
		t.Fatalf("prior executable changed: %q, %v", data, err)
	}

	// A damaged digest path cannot be overwritten or launched.
	if err := os.WriteFile(second, []byte("damaged"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareExecutable(state, source); err == nil || !strings.Contains(err.Error(), "fails source digest") {
		t.Fatalf("damaged executable accepted: %v", err)
	}
	if data, err := os.ReadFile(second); err != nil || string(data) != "damaged" {
		t.Fatalf("damaged path overwritten: %q, %v", data, err)
	}
}

// TestCopyExecutableRejectsBundleState prevents a configured state root from
// placing the daemon inside the bundle that an app update replaces.
func TestCopyExecutableRejectsBundleState(t *testing.T) {
	root := daemonTestRoot(t)
	bundle := filepath.Join(root, "Spacewave.app")
	state := filepath.Join(bundle, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "spacewave")
	if err := os.WriteFile(source, []byte("source"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareExecutable(state, source); err == nil || !strings.Contains(err.Error(), "inside application bundle") {
		t.Fatalf("bundle-local state accepted: %v", err)
	}
}
