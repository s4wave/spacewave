//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	spacewave_release "github.com/s4wave/spacewave/core/release"
	"github.com/sirupsen/logrus"
)

// TestApplicationReleaseWithoutCLI stages a real manifest through the launcher.
func TestApplicationReleaseWithoutCLI(t *testing.T) {
	// Publish a desktop-only application into the existing release-world testbed.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	platform := nativeTestPlatformID()
	ws := buildReleaseMetadataTestWorld(t, ctx, "alpha", platform)
	manifest := writeReleaseManifestTestBlockWithBinary(t, ctx, ws, "release/manifests/orbit", "orbit-desktop", platform, 2, "desktop payload")
	metadata := testReleaseMetadata("alpha", platform, manifest.GetManifestRef().GetRootRef())
	metadata.ProjectId = "orbit"
	metadata.BrowserShell = nil
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifest}
	metadataRef := writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataObjectKey("alpha"), metadata)
	writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataDirectoryObjectKey, &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{ChannelKey: "alpha", ReleaseMetadataRef: metadataRef}},
	})

	// Resolve the release using the same bus lookup and staging path as production.
	b := inmem.NewBus(cdc.NewController(ctx, le))
	release, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	dir := t.TempDir()
	sidecar := filepath.Join(dir, managedCLIReleaseSidecarFilename)
	if err := os.WriteFile(sidecar, []byte("obsolete cli"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctrl := &Controller{
		le:              le,
		bus:             b,
		conf:            &Config{ProjectId: "orbit", EntrypointManifestId: "orbit-desktop", DisableCliUpdate: true},
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](&spacewave_launcher.LauncherInfo{}),
		fetchStatusCtr:  ccontainer.NewCContainer[*spacewave_launcher.FetchStatus](&spacewave_launcher.FetchStatus{}),
		stagingDirFunc:  func() (string, error) { return dir, nil },
	}
	if err := ctrl.refreshReleaseMetadataStatus(ctx, &spacewave_launcher.DistConfig{ProjectId: "orbit", Rev: 1, ChannelKey: "alpha"}); err != nil {
		t.Fatal(err)
	}

	// The desktop becomes installable without a CLI or a stale CLI discovery file.
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_STAGED {
		t.Fatalf("update phase = %v: %s", state.GetPhase(), state.GetErrorMessage())
	}
	dat, err := os.ReadFile(state.GetStagedPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(dat) != "desktop payload" {
		t.Fatalf("staged desktop = %q", dat)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("obsolete CLI sidecar remains: %v", err)
	}
	if got := ctrl.fetchStatusCtr.GetValue().SelectedCLIManifestID; got != "" {
		t.Fatalf("desktop-only release selected CLI %q", got)
	}

	// Rechecking after replacement must not offer the same executable again.
	installed := filepath.Join(t.TempDir(), "installed.exe")
	if err := os.WriteFile(installed, dat, 0o755); err != nil {
		t.Fatal(err)
	}
	ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) {
		return installed, false, "", nil
	}
	if err := ctrl.refreshReleaseMetadataStatus(ctx, &spacewave_launcher.DistConfig{ProjectId: "orbit", Rev: 1, ChannelKey: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetUpdateState().GetPhase() == spacewave_launcher.UpdatePhase_UpdatePhase_STAGED {
		t.Fatal("offered the installed executable as an update")
	}
	if ctrl.fetchStatusCtr.GetValue().ReleaseMetadataOutcome != "current" {
		t.Fatal("installed release was not recognized")
	}

	// Application releases must not share Spacewave's version staging directory.
	dataRoot := t.TempDir()
	t.Setenv("ORBIT_DATA_DIR", dataRoot)
	ctrl.stagingDirFunc = nil
	if staging, err := ctrl.resolveStagingDir(); err != nil || staging != filepath.Join(dataRoot, "updates") {
		t.Fatalf("application staging directory = %q: %v", staging, err)
	}

	// Enabling the companion again must reject a release that does not include it.
	ctrl.conf.DisableCliUpdate = false
	if _, _, err := ctrl.conf.SelectReleaseManifests(metadata, platform); err == nil {
		t.Fatal("required CLI was silently omitted")
	}
	ctrl.conf.DisableCliUpdate = true
	metadata.ProjectId = "another-app"
	if _, _, err := ctrl.conf.SelectReleaseManifests(metadata, platform); err == nil {
		t.Fatal("accepted release metadata from another application")
	}
}
