//go:build !js

package spacewave_launcher_controller

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// TestStagedManifestRoundtrip verifies the sidecar proto marshalling path:
// write + read produces the exact same fields, and an absent file is not an
// error (caller treats it as "nothing staged").
func TestStagedManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()

	if got, err := readStagedManifest(dir); err != nil {
		t.Fatalf("read on empty dir: unexpected error %v", err)
	} else if got != nil {
		t.Fatalf("read on empty dir: expected nil, got %+v", got)
	}

	want := &spacewave_launcher.StagedManifest{
		Version:       "v0.1.2",
		Path:          filepath.Join(dir, "Spacewave-v0.1.2.app"),
		SignatureHash: []byte{0xde, 0xad, 0xbe, 0xef},
	}
	if err := writeStagedManifest(dir, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readStagedManifest(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.GetVersion() != want.GetVersion() ||
		got.GetPath() != want.GetPath() ||
		!bytes.Equal(got.GetSignatureHash(), want.GetSignatureHash()) {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", got, want)
	}
}

// TestStagedManifestCorrupt ensures that a garbled manifest produces an
// error so the caller wipes the staging tree instead of trusting junk.
func TestStagedManifestCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stagedManifestFilename), []byte("not-a-proto"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	if _, err := readStagedManifest(dir); err == nil {
		t.Fatal("expected unmarshal error on corrupt manifest, got nil")
	}
}

// TestStagedVersionFreshPassthrough encodes the "staged matches incoming
// DistConfig" freshness branch: when the persisted version equals the target
// entrypoint version and the path still exists, the check is a no-op and we
// keep the existing STAGED state.
func TestStagedVersionFreshPassthrough(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, "Spacewave-v1.2.3.app")
	if err := os.MkdirAll(stagedPath, 0o755); err != nil {
		t.Fatalf("seed staged path: %v", err)
	}
	want := &spacewave_launcher.StagedManifest{
		Version: "v1.2.3",
		Path:    stagedPath,
	}
	if err := writeStagedManifest(dir, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readStagedManifest(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	targetVersion := "v1.2.3"
	fresh := got.GetVersion() == targetVersion
	if !fresh {
		t.Fatalf("expected fresh match, got staged=%q target=%q", got.GetVersion(), targetVersion)
	}
	if _, err := os.Stat(got.GetPath()); err != nil {
		t.Fatalf("staged path should still exist: %v", err)
	}
}

// TestStagedVersionFreshnessMatrix covers the three freshness branches the
// updater must distinguish before deciding whether to keep, wipe, or skip the
// staged tree: fresh (keep), stale (wipe + re-download), missing (skip). IS-6
// regression guard: a stale STAGED must never mask a newer release.
func TestStagedVersionFreshnessMatrix(t *testing.T) {
	const targetVersion = "v1.2.4"

	cases := []struct {
		name         string
		stagedExists bool
		stagedVer    string
		wantFresh    bool
		wantStale    bool
		wantMissing  bool
	}{
		{name: "fresh", stagedExists: true, stagedVer: targetVersion, wantFresh: true},
		{name: "stale", stagedExists: true, stagedVer: "v1.2.3", wantStale: true},
		{name: "missing", stagedExists: false, wantMissing: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.stagedExists {
				stagedPath := filepath.Join(dir, "Spacewave-"+tc.stagedVer+".app")
				if err := os.MkdirAll(stagedPath, 0o755); err != nil {
					t.Fatalf("seed staged path: %v", err)
				}
				if err := writeStagedManifest(dir, &spacewave_launcher.StagedManifest{
					Version: tc.stagedVer,
					Path:    stagedPath,
				}); err != nil {
					t.Fatalf("write: %v", err)
				}
			}

			got, err := readStagedManifest(dir)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			if tc.wantMissing {
				if got != nil {
					t.Fatalf("missing case: expected nil manifest, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected manifest, got nil")
			}
			if _, err := os.Stat(got.GetPath()); err != nil {
				t.Fatalf("staged path should exist: %v", err)
			}

			fresh := got.GetVersion() == targetVersion
			if fresh != tc.wantFresh {
				t.Fatalf("fresh = %v, want %v (staged=%q target=%q)",
					fresh, tc.wantFresh, got.GetVersion(), targetVersion)
			}
			if tc.wantStale {
				if err := os.RemoveAll(got.GetPath()); err != nil {
					t.Fatalf("wipe staged path: %v", err)
				}
				if err := removeStagedManifest(dir); err != nil {
					t.Fatalf("wipe manifest: %v", err)
				}
				after, err := readStagedManifest(dir)
				if err != nil {
					t.Fatalf("read after wipe: %v", err)
				}
				if after != nil {
					t.Fatalf("expected manifest removed, got %+v", after)
				}
			}
		})
	}
}

// TestStagedVersionStaleMismatch encodes the wipe branch: persisted version
// differs from target, caller must wipe both the staged .app and the sidecar
// before re-entering DOWNLOADING.
func TestStagedVersionStaleMismatch(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, "Spacewave-v1.2.3.app")
	if err := os.MkdirAll(stagedPath, 0o755); err != nil {
		t.Fatalf("seed staged path: %v", err)
	}
	if err := writeStagedManifest(dir, &spacewave_launcher.StagedManifest{
		Version: "v1.2.3",
		Path:    stagedPath,
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readStagedManifest(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	targetVersion := "v1.2.4"
	if got.GetVersion() == targetVersion {
		t.Fatalf("setup: versions should differ")
	}

	// Caller's wipe behaviour: remove staged path and manifest.
	if err := os.RemoveAll(got.GetPath()); err != nil {
		t.Fatalf("wipe staged path: %v", err)
	}
	if err := removeStagedManifest(dir); err != nil {
		t.Fatalf("wipe manifest: %v", err)
	}

	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("expected staged path removed, got err=%v", err)
	}
	after, err := readStagedManifest(dir)
	if err != nil {
		t.Fatalf("read after wipe: %v", err)
	}
	if after != nil {
		t.Fatalf("expected manifest removed, got %+v", after)
	}
}
