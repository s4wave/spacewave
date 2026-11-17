//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// TestStagingKeepsOneRelease stages three versions and finds one version
// folder, while links, stray files and anything outside the staging
// directory stay.
func TestStagingKeepsOneRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows hosts")
	}
	ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
	installed := filepath.Join(t.TempDir(), "installed")
	if err := os.WriteFile(installed, []byte("older daemon"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) { return installed, false, "", nil }

	// A canary outside the staging directory, reached by a link named like
	// a version, by a link inside an old version folder, and by a version
	// name that climbs out of the staging directory.
	outside := t.TempDir()
	canary := filepath.Join(outside, "canary")
	if err := os.WriteFile(canary, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(stagingDir, "0.0.9")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stagingDir, "0.0.1", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(stagingDir, "0.0.1", "dist", "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "0.0.8"), []byte("a file"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Stage three versions in turn.
	for _, version := range []string{"0.1.0", "0.1.1", "0.1.2"} {
		metadata.Version = version
		if err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), manifestRef, cliManifestRef); err != nil {
			t.Fatalf("stage %s: %v", version, err)
		}
		if got := versionFolders(t, stagingDir); !slices.Equal(got, []string{version}) {
			t.Fatalf("after staging %s the folders are %q", version, got)
		}
	}

	// A version that climbs out is refused and removes nothing.
	metadata.Version = "../" + filepath.Base(outside)
	if err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), manifestRef, cliManifestRef); err == nil {
		t.Fatal("a climbing version staged")
	}
	if got := versionFolders(t, stagingDir); !slices.Equal(got, []string{"0.1.2"}) {
		t.Fatalf("after the refused version the folders are %q", got)
	}

	// The links, the file, the sidecar and the canary stay.
	for _, name := range []string{"0.0.9", "0.0.8", managedCLIReleaseSidecarFilename} {
		if _, err := os.Lstat(filepath.Join(stagingDir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if data, err := os.ReadFile(canary); err != nil || string(data) != "keep" {
		t.Fatalf("canary = %q, %v", data, err)
	}
}

// TestStagingKeepsReleasesInUse keeps an accepted daemon handoff, a daemon
// running from a staged copy, every copy when the daemon cannot be located,
// and every copy when the caller keeps them.
func TestStagingKeepsReleasesInUse(t *testing.T) {
	cases := []struct {
		name string
		// prepare sets up the controller once 0.1.0 and 0.1.1 are staged.
		prepare func(ctrl *Controller, stagingDir string)
		want    []string
	}{{
		name: "accepted handoff",
		prepare: func(ctrl *Controller, _ string) {
			_, _, _ = ctrl.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
				info.DaemonUpdateState = &spacewave_launcher.UpdateState{
					Phase:   spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING,
					Version: "0.1.0",
				}
				return true, nil
			})
		},
		want: []string{"0.1.0", "0.1.2"},
	}, {
		name: "daemon runs from a staged copy",
		prepare: func(ctrl *Controller, stagingDir string) {
			running := filepath.Join(stagingDir, "0.1.1", "cli-dist", "spacewave")
			ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) { return running, false, "", nil }
		},
		want: []string{"0.1.1", "0.1.2"},
	}, {
		name: "daemon unknown",
		prepare: func(ctrl *Controller, _ string) {
			ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) {
				return "", false, "", bldr_plugin.ErrHostExecutableUnknown
			}
		},
		want: []string{"0.1.0", "0.1.1", "0.1.2"},
	}, {
		name:    "kept by the caller",
		prepare: func(ctrl *Controller, _ string) { ctrl.KeepStagedReleases() },
		want:    []string{"0.1.0", "0.1.1", "0.1.2"},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Stage two versions with pruning off, then prepare the case.
			ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
			installed := filepath.Join(t.TempDir(), "installed")
			if err := os.WriteFile(installed, []byte("older daemon"), 0o755); err != nil {
				t.Fatal(err)
			}
			ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) { return installed, false, "", nil }
			ctrl.keepStagedReleases = true
			for _, version := range []string{"0.1.0", "0.1.1"} {
				metadata.Version = version
				if err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), manifestRef, cliManifestRef); err != nil {
					t.Fatalf("stage %s: %v", version, err)
				}
			}
			ctrl.keepStagedReleases = false
			tc.prepare(ctrl, stagingDir)

			// Stage the third and compare what stays.
			metadata.Version = "0.1.2"
			if err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), manifestRef, cliManifestRef); err != nil {
				t.Fatalf("stage 0.1.2: %v", err)
			}
			if got := versionFolders(t, stagingDir); !slices.Equal(got, tc.want) {
				t.Fatalf("folders = %q; want %q", got, tc.want)
			}
		})
	}
}

// versionFolders lists the directories, not links, at the top of dir.
func versionFolders(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var folders []string
	for _, entry := range entries {
		if entry.Type().IsDir() {
			folders = append(folders, entry.Name())
		}
	}
	return folders
}
