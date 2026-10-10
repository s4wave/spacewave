//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	spacewave_release "github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

func TestReadSelectedReleaseMetadata(t *testing.T) {
	// Build the native channel metadata fixture.
	ctx := context.Background()
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())

	// Resolve the selected channel and its platform support.
	metadata, err := readSelectedReleaseMetadata(ctx, ws, "stable")
	if err != nil {
		t.Fatalf("readSelectedReleaseMetadata() error = %v", err)
	}
	if metadata.GetChannelKey() != "stable" {
		t.Fatalf("channel key = %q", metadata.GetChannelKey())
	}
	if !releaseMetadataSupportsPlatform(metadata, nativeTestPlatformID()) {
		t.Fatalf("metadata does not support native platform")
	}
}

// TestDaemonApplyingSelectionSurvivesReleaseRefresh keeps the accepted CLI
// selection visible while refresh clears and stages a later release. The app
// target still follows that later release independently.
func TestDaemonApplyingSelectionSurvivesReleaseRefresh(t *testing.T) {
	// Seed independently accepted daemon and desktop selections.
	accepted := &spacewave_launcher.UpdateState{
		Phase:              spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING,
		Version:            "0.1.0",
		StagedPath:         "/selected/0.1.0/spacewave",
		StagedSha256:       strings.Repeat("a", 64),
		Target:             desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
		ArtifactManifestId: cliEntrypointManifestID,
	}
	ctrl := &Controller{launcherInfoCtr: ccontainer.NewCContainer(&spacewave_launcher.LauncherInfo{
		DaemonUpdateState: accepted,
		UpdateState: &spacewave_launcher.UpdateState{
			Phase:   spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
			Version: "0.1.0",
			Target:  desktop_update.UpdateTarget_UPDATE_TARGET_APP,
		},
	})}

	// Refresh clears an old selection before staging the next release. A
	// missing daemon target can also clear it after staging completes.
	ctrl.clearDaemonUpdateState()
	ctrl.setDaemonUpdateStaged("0.2.0", cliEntrypointManifestID, "/selected/0.2.0/spacewave", strings.Repeat("b", 64))
	ctrl.setUpdateStaged("0.2.0", "/selected/0.2.0/Spacewave.app")
	ctrl.clearDaemonUpdateState()
	info := ctrl.launcherInfoCtr.GetValue()
	if !info.GetDaemonUpdateState().EqualVT(accepted) {
		t.Fatalf("refresh replaced accepted daemon selection: %v", info.GetDaemonUpdateState())
	}
	if state := info.GetUpdateState(); state.GetVersion() != "0.2.0" || state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatalf("app selection did not advance independently: %v", state)
	}

	// An explicit handoff failure releases the hold on the old selection.
	if !ctrl.setAcceptedDaemonUpdateError(accepted, "selected copy failed") {
		t.Fatal("accepted failure did not publish")
	}
	ctrl.setDaemonUpdateStaged("0.2.0", cliEntrypointManifestID, "/selected/0.2.0/spacewave", strings.Repeat("b", 64))
	if state := ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState(); state.GetVersion() != "0.2.0" || state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatalf("later daemon selection did not stage after failure: %v", state)
	}
	ctrl.clearDaemonUpdateState()
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState() != nil {
		t.Fatal("clear retained a staged daemon selection after failure")
	}
}

func TestReadSelectedReleaseMetadataErrors(t *testing.T) {
	// Build a channel that supports no native desktop.
	ctx := context.Background()
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", "desktop/other/arch")

	// Require explicit channel selection and platform support.
	if _, err := readSelectedReleaseMetadata(ctx, ws, "beta"); err == nil {
		t.Fatal("expected missing channel error")
	}
	metadata, err := readSelectedReleaseMetadata(ctx, ws, "stable")
	if err != nil {
		t.Fatalf("readSelectedReleaseMetadata() error = %v", err)
	}
	if releaseMetadataSupportsPlatform(metadata, nativeTestPlatformID()) {
		t.Fatalf("metadata unexpectedly supports native platform")
	}
}

func TestSelectReleaseManifestRefRequiresNativeEntrypointIdentity(t *testing.T) {
	// Build a native selection with an unrelated plugin.
	platformID := nativeTestPlatformID()
	valid := testManifestRef(nativeEntrypointManifestID, platformID, 1)
	selected, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{
			testManifestRef("spacewave-plugin", platformID, 1),
			valid,
		},
	}, platformID)
	if err != nil {
		t.Fatalf("selectReleaseManifestRef() error = %v", err)
	}
	if selected != valid {
		t.Fatal("selector did not return the native entrypoint ref")
	}

	// Reject a missing or ambiguous native entrypoint.
	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{testManifestRef("spacewave-plugin", platformID, 1)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "non-entrypoint native manifests") {
		t.Fatalf("wrong-identity error = %v", err)
	}

	// Reject duplicate native candidates.
	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{valid, testManifestRef(nativeEntrypointManifestID, platformID, 2)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "duplicate native entrypoint manifest") {
		t.Fatalf("duplicate error = %v", err)
	}

	// Reject a native candidate for another platform.
	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{testManifestRef(nativeEntrypointManifestID, "desktop/other/arch", 1)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "missing native entrypoint manifest") {
		t.Fatalf("missing error = %v", err)
	}
}

func TestSelectReleaseManifestsRequiresCLIEntrypointIdentity(t *testing.T) {
	// Select both entrypoints from metadata that also carries a plugin.
	platformID := nativeTestPlatformID()
	desktop := testManifestRef(nativeEntrypointManifestID, platformID, 1)
	cli := testManifestRef(cliEntrypointManifestID, platformID, 2)
	conf := &Config{}
	selectedDesktop, selectedCLI, err := conf.SelectReleaseManifests(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{
			testManifestRef("spacewave-plugin", platformID, 1),
			desktop,
			cli,
		},
	}, platformID)
	if err != nil {
		t.Fatalf("SelectReleaseManifests() error = %v", err)
	}
	if selectedDesktop != desktop || selectedCLI != cli {
		t.Fatal("selector did not return the desktop and cli entrypoint refs")
	}

	// A missing or duplicated CLI entrypoint fails the selection.
	if _, _, err := conf.SelectReleaseManifests(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{desktop},
	}, platformID); err == nil || !strings.Contains(err.Error(), "missing spacewave-cli") {
		t.Fatalf("wrong-identity error = %v", err)
	}
	if _, _, err := conf.SelectReleaseManifests(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{desktop, cli, testManifestRef(cliEntrypointManifestID, platformID, 3)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "duplicate cli entrypoint manifest") {
		t.Fatalf("duplicate error = %v", err)
	}

	// The CLI entrypoint never satisfies the desktop selector.
	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{cli},
	}, platformID); err == nil || !strings.Contains(err.Error(), "missing spacewave-dist") {
		t.Fatalf("cli should not satisfy native selector: %v", err)
	}
}

func TestCheckoutReleaseManifestStagesDist(t *testing.T) {
	// Build a filesystem-backed native release.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Checkout the manifest through the real release World.
	manifestRef := writeReleaseManifestTestBlock(t, ctx, ws, "release/manifests/native", src)
	out := t.TempDir()
	manifest, err := checkoutReleaseManifest(
		ctx,
		le,
		ws,
		manifestRef,
		filepath.Join(out, "dist"),
		filepath.Join(out, "assets"),
	)
	if err != nil {
		t.Fatalf("checkoutReleaseManifest() error = %v", err)
	}
	if manifest.GetEntrypoint() != "spacewave" {
		t.Fatalf("entrypoint = %q", manifest.GetEntrypoint())
	}
	got, err := os.ReadFile(filepath.Join(out, "dist", "spacewave"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(got) != "binary" {
		t.Fatalf("staged binary = %q", string(got))
	}
}

func TestRefreshReleaseMetadataStatusStagesWithoutR2Media(t *testing.T) {
	// Publish both executable roots for the native release.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
	manifestRef := writeReleaseDesktopArtifactTestBlock(t, ctx, ws, "release/manifests/native", nativeEntrypointManifestID, nativeTestPlatformID(), 1, "binary")
	cliManifestRef := writeReleaseManifestTestBlockWithBinary(
		t,
		ctx,
		ws,
		"release/manifests/cli",
		cliEntrypointManifestID,
		nativeTestPlatformID(),
		2,
		"cli",
	)

	// Publish the selected channel metadata.
	metadata := testReleaseMetadata("stable", nativeTestPlatformID(), manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef, cliManifestRef}
	metadataRef := writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataObjectKey("stable"), metadata)
	writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataDirectoryObjectKey, &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{
			ChannelKey:         "stable",
			ReleaseMetadataRef: metadataRef,
		}},
	})

	// Mount the fixture World on the launcher bus.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Stage under an isolated launcher directory.
	stagingDir := t.TempDir()
	ctrl := &Controller{
		le:          le,
		bus:         b,
		distPeerIDs: releaseAuthorizationTestPins(t),
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:  "spacewave",
					Rev:        1,
					ChannelKey: "stable",
				},
			},
		),
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
	}
	ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig())

	// Require a staged desktop target for the selected manifest.
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatalf("phase = %v error=%q", state.GetPhase(), state.GetErrorMessage())
	}
	if state.GetTarget() != desktop_update.UpdateTarget_UPDATE_TARGET_APP || state.GetArtifactManifestId() != nativeEntrypointManifestID {
		t.Fatalf("desktop target = %v artifact = %q", state.GetTarget(), state.GetArtifactManifestId())
	}
	wantStagedPath := filepath.Join(stagingDir, "0.1.0", "dist", "spacewave")
	if runtime.GOOS == "darwin" {
		wantStagedPath = filepath.Join(stagingDir, "0.1.0", "dist", "Spacewave.app")
	}
	if state.GetStagedPath() != wantStagedPath {
		t.Fatalf("staged path = %q", state.GetStagedPath())
	}

	// Read the staged installed-app executable.
	appExecutable := state.GetStagedPath()
	if runtime.GOOS == "darwin" {
		appExecutable = filepath.Join(appExecutable, "Contents", "MacOS", "spacewave")
	}
	got, err := os.ReadFile(appExecutable)
	if err != nil {
		t.Fatal(err.Error())
	}
	if runtime.GOOS != "darwin" && string(got) != "binary" {
		t.Fatalf("staged binary = %q", string(got))
	}

	// Require staged resolution diagnostics.
	if outcome := ctrl.launcherInfoCtr.GetValue().GetFetchStatus().GetReleaseMetadataOutcome(); outcome != spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_STAGED {
		t.Fatalf("release metadata outcome = %v, want staged", outcome)
	}

	// Check the selected desktop manifest identity.
	fetchStatus := ctrl.launcherInfoCtr.GetValue().GetFetchStatus()
	if fetchStatus.SelectedEntrypointManifestId != nativeEntrypointManifestID {
		t.Fatalf("selected entrypoint id = %q", fetchStatus.SelectedEntrypointManifestId)
	}
	if fetchStatus.SelectedEntrypointPlatformId != nativeTestPlatformID() {
		t.Fatalf("selected entrypoint platform = %q", fetchStatus.SelectedEntrypointPlatformId)
	}
	if fetchStatus.SelectedEntrypointManifestRev != 1 {
		t.Fatalf("selected entrypoint rev = %d", fetchStatus.SelectedEntrypointManifestRev)
	}
	if fetchStatus.SelectedEntrypointManifestRef == "" {
		t.Fatal("selected entrypoint ref is empty")
	}

	// Check the selected companion manifest identity.
	if fetchStatus.SelectedCliManifestId != cliEntrypointManifestID {
		t.Fatalf("selected CLI entrypoint id = %q", fetchStatus.SelectedCliManifestId)
	}
	if fetchStatus.SelectedCliPlatformId != nativeTestPlatformID() {
		t.Fatalf("selected CLI entrypoint platform = %q", fetchStatus.SelectedCliPlatformId)
	}
	if fetchStatus.SelectedCliManifestRev != 2 {
		t.Fatalf("selected CLI entrypoint rev = %d", fetchStatus.SelectedCliManifestRev)
	}
	if fetchStatus.SelectedCliManifestRef == "" {
		t.Fatal("selected CLI entrypoint ref is empty")
	}
	if fetchStatus.SelectedCliBinaryPath != filepath.Join(stagingDir, "0.1.0", "cli-dist", "spacewave") {
		t.Fatalf("selected CLI binary path = %q", fetchStatus.SelectedCliBinaryPath)
	}

	// Read the companion discovery record.
	sidecar, err := os.ReadFile(filepath.Join(stagingDir, managedCLIReleaseSidecarFilename))
	if err != nil {
		t.Fatal(err.Error())
	}
	sidecarText := string(sidecar)
	if !strings.Contains(sidecarText, `"manifest_id": "spacewave-cli"`) {
		t.Fatalf("sidecar missing CLI manifest id: %s", sidecarText)
	}
	if !strings.Contains(sidecarText, `"manifest_rev": 2`) {
		t.Fatalf("sidecar missing CLI manifest rev: %s", sidecarText)
	}
	if !strings.Contains(sidecarText, `"binary_path": `) {
		t.Fatalf("sidecar missing binary path: %s", sidecarText)
	}

	// Verify the staged companion payload and daemon target.
	cliBinary, err := os.ReadFile(fetchStatus.SelectedCliBinaryPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(cliBinary) != "cli" {
		t.Fatalf("staged CLI binary = %q", string(cliBinary))
	}
	daemonState := ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState()
	if daemonState.GetTarget() != desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON || daemonState.GetArtifactManifestId() != cliEntrypointManifestID || daemonState.GetStagedPath() != fetchStatus.SelectedCliBinaryPath {
		t.Fatalf("daemon artifact selection = %#v", daemonState)
	}

	// Require the daemon digest to match its staged executable.
	digest, err := stagedExecutableSHA256(fetchStatus.SelectedCliBinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if daemonState.GetStagedSha256() != digest {
		t.Fatalf("staged daemon digest = %q, want %q", daemonState.GetStagedSha256(), digest)
	}
	if fetchStatus.ReleaseWorldHeadRef == "" {
		t.Fatal("release world head ref is empty")
	}

	// Matching CLI bytes suppress only the daemon target, not the desktop app.
	installedCLI := filepath.Join(t.TempDir(), "spacewave-daemon")
	if err := os.WriteFile(installedCLI, cliBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) {
		return installedCLI, false, "", nil
	}
	if err := ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig()); err != nil {
		t.Fatal(err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState() != nil {
		t.Fatal("offered matching daemon bytes as an update")
	}
	if ctrl.launcherInfoCtr.GetValue().GetUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatal("matching daemon bytes hid the app update")
	}

	// Matching installed-app bytes must still expose the desktop update.
	if err := os.WriteFile(installedCLI, got, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig()); err != nil {
		t.Fatal(err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatal("matching app and daemon executable bytes hid the app update")
	}
	if ctrl.launcherInfoCtr.GetValue().GetFetchStatus().GetReleaseMetadataOutcome() != spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_STAGED {
		t.Fatal("matching daemon executable misclassified the app as current")
	}

	// A plugin whose host did not report its executable offers no daemon
	// update rather than comparing against its own plugin binary.
	ctrl.currentExecutableBundleFunc = nil
	t.Setenv("BLDR_PLUGIN_START_INFO", "plugin")
	t.Setenv(bldr_plugin.HostExecutableEnv, "")
	if err := ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig()); err != nil {
		t.Fatal(err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState() != nil {
		t.Fatal("offered a daemon update without the host executable")
	}
	if ctrl.launcherInfoCtr.GetValue().GetUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatal("unknown host executable hid the app update")
	}

	// A plugin compares its host executable with the staged daemon.
	t.Setenv(bldr_plugin.HostExecutableEnv, installedCLI)
	if err := ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig()); err != nil {
		t.Fatal(err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState() == nil {
		t.Fatal("host executable with different bytes offered no daemon update")
	}

	// A daemon running from an app bundle copy moves to the same executable in
	// the staged app bundle, and matching bundle bytes offer no daemon update.
	if runtime.GOOS != "darwin" {
		return
	}
	installedApp := filepath.Join(t.TempDir(), "Spacewave.app")
	installedExecutable := filepath.Join(installedApp, "Contents", "MacOS", "spacewave")
	if err := os.MkdirAll(filepath.Dir(installedExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installedExecutable, []byte("previous app daemon"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) {
		return installedExecutable, true, installedApp, nil
	}
	if err := ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig()); err != nil {
		t.Fatal(err)
	}

	// Require the daemon selection to name the app bundle executable.
	bundleState := ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState()
	if bundleState.GetArtifactManifestId() != nativeEntrypointManifestID || bundleState.GetStagedPath() != appExecutable {
		t.Fatalf("bundle daemon selection = %#v", bundleState)
	}

	// Suppress a daemon update once the installed bundle matches.
	if err := os.WriteFile(installedExecutable, got, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig()); err != nil {
		t.Fatal(err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState() != nil {
		t.Fatal("offered matching app bundle daemon bytes as an update")
	}
}

func TestRefreshReleaseMetadataStatusClearsStaleReleaseWorldHeadOnError(t *testing.T) {
	// Build the channel metadata and release World.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())

	// Mount the fixture World through the launcher bus.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Seed stale release selection diagnostics.
	ctrl := &Controller{
		le:  le,
		bus: b,
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				FetchStatus: &spacewave_launcher.FetchStatus{
					ReleaseWorldHeadRef:           "previous-head",
					SelectedEntrypointManifestRef: "previous-manifest",
					SelectedCliManifestRef:        "previous-cli-manifest",
				},
			},
		),
		stagingDirFunc: func() (string, error) { return t.TempDir(), nil },
	}

	// Refresh a missing channel and require stale diagnostics to clear.
	err = ctrl.refreshReleaseMetadataStatus(ctx, &spacewave_launcher.DistConfig{
		ProjectId:  "spacewave",
		Rev:        1,
		ChannelKey: "missing",
	})
	if err == nil {
		t.Fatal("expected missing channel error")
	}
	fetchStatus := ctrl.launcherInfoCtr.GetValue().GetFetchStatus()
	if fetchStatus.GetReleaseMetadataOutcome() != spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_ERROR {
		t.Fatalf("release metadata outcome = %v, want error", fetchStatus.ReleaseMetadataOutcome)
	}
	if fetchStatus.ReleaseWorldHeadRef != "" {
		t.Fatalf("release world head ref = %q, want cleared", fetchStatus.ReleaseWorldHeadRef)
	}
	if fetchStatus.SelectedEntrypointManifestRef != "" {
		t.Fatalf("selected entrypoint ref = %q, want cleared", fetchStatus.SelectedEntrypointManifestRef)
	}
	if fetchStatus.SelectedCliManifestRef != "" {
		t.Fatalf("selected CLI entrypoint ref = %q, want cleared", fetchStatus.SelectedCliManifestRef)
	}
}

func TestRefreshReleaseMetadataStatusRejectsDirectoryEntrypoint(t *testing.T) {
	// Publish a directory where the executable should be.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "spacewave"), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	if err := os.WriteFile(filepath.Join(src, "spacewave", "binary"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err.Error())
	}

	// Publish the independent companion executable.
	manifestRef := writeReleaseManifestTestBlock(t, ctx, ws, "release/manifests/native", src)
	cliManifestRef := writeReleaseManifestTestBlockWithBinary(
		t,
		ctx,
		ws,
		"release/manifests/cli",
		cliEntrypointManifestID,
		nativeTestPlatformID(),
		2,
		"cli",
	)
	metadata := testReleaseMetadata("stable", nativeTestPlatformID(), manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef, cliManifestRef}
	metadataRef := writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataObjectKey("stable"), metadata)
	writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataDirectoryObjectKey, &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{
			ChannelKey:         "stable",
			ReleaseMetadataRef: metadataRef,
		}},
	})

	// Mount the fixture World through the launcher bus.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Refresh the malformed installed-app candidate.
	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(t, le, b, stagingDir)
	err = ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig())
	if err == nil {
		t.Fatal("expected directory entrypoint error")
	}
	want := "staged directory entrypoint must be a .app bundle"
	if isDarwinDesktopPlatform(nativeTestPlatformID()) {
		want = "darwin installed-app update must stage a signed .app bundle"
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q", err.Error())
	}

	// Require an error state and remove the rejected checkout.
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR {
		t.Fatalf("phase = %v, want ERROR", state.GetPhase())
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "0.1.0")); !os.IsNotExist(err) {
		t.Fatalf("stage root should be removed, stat err = %v", err)
	}
}

// TestReleaseMetadataRoutineRefreshesThenWaitsForReleaseWorld proves a routine
// that starts before the release World mounts asks the mount to fetch its root
// and resolves the release once the mount publishes its engine.
func TestReleaseMetadataRoutineRefreshesThenWaitsForReleaseWorld(t *testing.T) {
	// Build a bounded release metadata scenario.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())

	// Publish both executable roots and selected metadata.
	manifestRef := writeReleaseDesktopArtifactTestBlock(t, ctx, ws, "release/manifests/native", nativeEntrypointManifestID, nativeTestPlatformID(), 1, "binary")
	cliManifestRef := writeReleaseManifestTestBlockWithBinary(
		t,
		ctx,
		ws,
		"release/manifests/cli",
		cliEntrypointManifestID,
		nativeTestPlatformID(),
		2,
		"cli",
	)
	metadata := testReleaseMetadata("stable", nativeTestPlatformID(), manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef, cliManifestRef}
	metadataRef := writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataObjectKey("stable"), metadata)
	writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataDirectoryObjectKey, &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{
			ChannelKey:         "stable",
			ReleaseMetadataRef: metadataRef,
		}},
	})

	// Mount the refresh service before the World appears.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	refresher := &releaseWorldRefreshTestController{refreshed: make(chan struct{}, 1)}
	relRefresh, err := b.AddController(ctx, refresher, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relRefresh()

	// Start metadata resolution with no mounted World.
	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(t, le, b, stagingDir)
	ctrl.releaseMetadataRoutine.SetContext(ctx, true)
	defer ctrl.releaseMetadataRoutine.ClearContext()

	// With no World mounted the routine asks the mount for a root fetch and
	// waits instead of failing into its retry backoff.
	select {
	case <-refresher.refreshed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Require the mounted World to produce a staged desktop.
	state := waitForUpdatePhase(t, ctrl, spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED)
	wantStagedPath := filepath.Join(stagingDir, "0.1.0", "dist", "spacewave")
	if runtime.GOOS == "darwin" {
		wantStagedPath = filepath.Join(stagingDir, "0.1.0", "dist", "Spacewave.app")
	}
	if state.GetStagedPath() != wantStagedPath {
		t.Fatalf("staged path = %q", state.GetStagedPath())
	}
}

func TestRefreshReleaseMetadataStatusRefreshesLaggingWorld(t *testing.T) {
	// Build metadata behind the incoming selector revision.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())

	// Mount the World and its refresh service.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Count refresh requests sent to the owning mount.
	refresher := &releaseWorldRefreshTestController{}
	relRefresh, err := b.AddController(ctx, refresher, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relRefresh()

	// The pushed selector names revision 2 while the mounted World holds 1.
	ctrl := newReleaseMetadataRoutineTestController(t, le, b, t.TempDir())
	ctrl.launcherInfoCtr.SetValue(&spacewave_launcher.LauncherInfo{
		DistConfig: &spacewave_launcher.DistConfig{ProjectId: "spacewave", Rev: 2, ChannelKey: "stable"},
	})
	err = ctrl.refreshCurrentReleaseMetadataStatus(ctx)
	if err == nil || !strings.Contains(err.Error(), "behind selector 2") {
		t.Fatalf("error = %v, want release world behind selector", err)
	}
	if refresher.calls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refresher.calls.Load())
	}
	if outcome := ctrl.launcherInfoCtr.GetValue().GetFetchStatus().GetReleaseMetadataOutcome(); outcome != spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_REFRESHING {
		t.Fatalf("release metadata outcome = %v, want refreshing", outcome)
	}
	if phase := ctrl.launcherInfoCtr.GetValue().GetUpdateState().GetPhase(); phase == spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR {
		t.Fatal("a lagging World must not surface an update error")
	}
}

func TestRefreshReleaseMetadataStatusErrorsWhenNativeManifestMissing(t *testing.T) {
	// Build metadata that lacks the native platform.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", "js")

	// Mount the fixture World through the launcher bus.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Seed a previous error and refresh the incompatible release.
	ctrl := newReleaseMetadataRoutineTestController(t, le, b, t.TempDir())
	ctrl.launcherInfoCtr.SetValue(&spacewave_launcher.LauncherInfo{
		DistConfig: ctrl.launcherInfoCtr.GetValue().GetDistConfig(),
		UpdateState: &spacewave_launcher.UpdateState{
			Phase:        spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR,
			ErrorMessage: "previous",
		},
	})
	err = ctrl.refreshCurrentReleaseMetadataStatus(ctx)
	if err == nil {
		t.Fatal("expected missing native manifest error")
	}
	want := "release metadata missing native entrypoint manifest " + nativeEntrypointManifestID + " for platform " + nativeTestPlatformID()
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}

	// Require the current error and diagnostics to identify the missing entrypoint.
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR {
		t.Fatalf("phase = %v, want ERROR", state.GetPhase())
	}
	if !strings.Contains(state.GetErrorMessage(), want) {
		t.Fatalf("error message = %q, want %q", state.GetErrorMessage(), want)
	}
	if outcome := ctrl.launcherInfoCtr.GetValue().GetFetchStatus().GetReleaseMetadataOutcome(); outcome != spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_ERROR {
		t.Fatalf("release metadata outcome = %v, want error", outcome)
	}
}

func TestRefreshReleaseMetadataStatusErrorsWhenCLIManifestMissing(t *testing.T) {
	// Build a release that omits its required companion.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	manifestRef := writeReleaseManifestTestBlock(t, ctx, ws, "release/manifests/native", src)

	// Publish only the native entrypoint in channel metadata.
	metadata := testReleaseMetadata("stable", nativeTestPlatformID(), manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef}
	metadataRef := writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataObjectKey("stable"), metadata)
	writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataDirectoryObjectKey, &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{
			ChannelKey:         "stable",
			ReleaseMetadataRef: metadataRef,
		}},
	})

	// Mount the fixture World through the launcher bus.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Refresh the release and require the missing companion error.
	ctrl := newReleaseMetadataRoutineTestController(t, le, b, t.TempDir())
	err = ctrl.refreshCurrentReleaseMetadataStatus(ctx)
	if err == nil {
		t.Fatal("expected missing CLI manifest error")
	}
	want := "missing " + cliEntrypointManifestID
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if outcome := ctrl.launcherInfoCtr.GetValue().GetFetchStatus().GetReleaseMetadataOutcome(); outcome != spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_ERROR {
		t.Fatalf("release metadata outcome = %v, want error", outcome)
	}
}

func TestStageReleaseManifestUpdateRejectsRawDarwinPayloadWithOutsideDaemon(t *testing.T) {
	// Build a raw Darwin executable outside an app bundle.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", "desktop/darwin/arm64")
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	manifestRef := writeReleaseManifestTestBlockWithMeta(
		t,
		ctx,
		ws,
		"release/manifests/native",
		src,
		nativeEntrypointManifestID,
		"desktop/darwin/arm64",
		1,
	)
	cliManifestRef := writeReleaseManifestTestBlockWithBinary(
		t,
		ctx,
		ws,
		"release/manifests/cli",
		cliEntrypointManifestID,
		"desktop/darwin/arm64",
		2,
		"cli",
	)

	// Mount the fixture World through the launcher bus.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Stage the release with a daemon outside the installed app.
	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(t, le, b, stagingDir)
	ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) {
		return filepath.Join(t.TempDir(), "daemon-bin", "spacewave"), false, "", nil
	}
	metadata := testReleaseMetadata("stable", "desktop/darwin/arm64", manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef, cliManifestRef}

	// Reject the raw payload and remove its staging directory.
	err = ctrl.stageReleaseManifestUpdate(ctx, metadata, "desktop/darwin/arm64", manifestRef, cliManifestRef)
	if err == nil {
		t.Fatal("expected raw Darwin installed-app payload error")
	}
	if !strings.Contains(err.Error(), "darwin installed-app update must stage a signed .app bundle") {
		t.Fatalf("error = %q", err.Error())
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "0.1.0")); !os.IsNotExist(err) {
		t.Fatalf("stage root should be removed, stat err = %v", err)
	}
}

func TestStageReleaseManifestUpdateRejectsPathLikeVersion(t *testing.T) {
	// Prepare a release with an escaping version path.
	ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
	metadata.Version = "../escape"

	// Reject the version before creating a directory outside staging.
	err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), manifestRef, cliManifestRef)
	if err == nil {
		t.Fatal("expected path-like release version error")
	}
	if !strings.Contains(err.Error(), "release version must be a local path segment") {
		t.Fatalf("error = %q", err.Error())
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "..", "escape")); !os.IsNotExist(err) {
		t.Fatalf("escaped stage root should not exist, stat err = %v", err)
	}
}

func TestStageReleaseManifestUpdateRejectsSymlinkedStagingRoot(t *testing.T) {
	// Prepare an existing version directory symlink.
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows hosts")
	}
	ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(stagingDir, "0.1.0")); err != nil {
		t.Fatal(err.Error())
	}

	// Reject the linked staging root without writing to its target.
	err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), manifestRef, cliManifestRef)
	if err == nil {
		t.Fatal("expected symlinked release staging root error")
	}
	if !strings.Contains(err.Error(), "release staging root must not be a symlink") {
		t.Fatalf("error = %q", err.Error())
	}
	if _, err := os.Stat(filepath.Join(outside, "cli-dist", "spacewave")); !os.IsNotExist(err) {
		t.Fatalf("outside cli checkout should not exist, stat err = %v", err)
	}
}

func TestStageReleaseManifestUpdateRejectsSymlinkedCheckoutRoot(t *testing.T) {
	// Prepare a linked companion checkout directory.
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows hosts")
	}
	ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
	stageRoot := filepath.Join(stagingDir, "0.1.0")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		t.Fatal(err.Error())
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(stageRoot, "cli-dist")); err != nil {
		t.Fatal(err.Error())
	}

	// Reject the checkout symlink without writing to its target.
	err := ctrl.stageReleaseManifestUpdate(ctx, metadata, nativeTestPlatformID(), manifestRef, cliManifestRef)
	if err == nil {
		t.Fatal("expected symlinked cli checkout root error")
	}
	if !strings.Contains(err.Error(), "release checkout root must not be a symlink") {
		t.Fatalf("error = %q", err.Error())
	}
	if _, err := os.Stat(filepath.Join(outside, "spacewave")); !os.IsNotExist(err) {
		t.Fatalf("outside cli checkout should not exist, stat err = %v", err)
	}
}

func TestStagedManifestEntrypointPathRejectsEscapes(t *testing.T) {
	// Require local entrypoint paths inside the checkout.
	distPath := filepath.Join(t.TempDir(), "dist")
	if _, err := stagedManifestEntrypointPath(distPath, "../spacewave"); err == nil {
		t.Fatal("expected parent escape error")
	}
	if _, err := stagedManifestEntrypointPath(distPath, "/spacewave"); err == nil {
		t.Fatal("expected absolute path error")
	}
	if _, err := stagedManifestEntrypointPath(distPath, `dir\spacewave`); err == nil {
		t.Fatal("expected backslash path error")
	}
	got, err := stagedManifestEntrypointPath(distPath, "bin/spacewave")
	if err != nil {
		t.Fatalf("stagedManifestEntrypointPath() error = %v", err)
	}
	if got != filepath.Join(distPath, "bin", "spacewave") {
		t.Fatalf("staged path = %q", got)
	}
}

func TestVerifyStagedExecutableRejectsSymlink(t *testing.T) {
	// Prepare a final executable path that is a symlink.
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows hosts")
	}
	stageRoot := t.TempDir()
	cliDistPath := filepath.Join(stageRoot, "cli-dist")
	if err := os.MkdirAll(cliDistPath, 0o755); err != nil {
		t.Fatal(err.Error())
	}
	outside := filepath.Join(t.TempDir(), "spacewave")
	if err := os.WriteFile(outside, []byte("outside"), 0o755); err != nil {
		t.Fatal(err.Error())
	}

	// Link the staged entrypoint to a file outside the checkout.
	stagedPath := filepath.Join(cliDistPath, "spacewave")
	if err := os.Symlink(outside, stagedPath); err != nil {
		t.Fatal(err.Error())
	}

	// Reject the linked executable and remove the checkout.
	err := verifyStagedExecutable(stageRoot, cliDistPath, stagedPath)
	if err == nil {
		t.Fatal("expected symlink entrypoint error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %q, want symlink", err.Error())
	}
	if _, err := os.Stat(stageRoot); !os.IsNotExist(err) {
		t.Fatalf("stage root should be removed, stat err = %v", err)
	}
}

func buildReleaseMetadataTestWorld(
	t *testing.T,
	ctx context.Context,
	channelKey string,
	platformID string,
) world.WorldState {
	// Build an isolated block-backed release World.
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	// Construct the World engine and register its cleanup.
	eng, err := world_block.NewEngine(ctx, le, ocs, nil, nil, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Fatal(err.Error())
		}
	})

	// Publish the initial release channel directory.
	ws := world.NewEngineWorldState(eng, true)
	ref := testBlockRef()
	metadata := testReleaseMetadata(channelKey, platformID, ref)
	metadataRef := writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataObjectKey(channelKey), metadata)
	directory := &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{
			ChannelKey:         channelKey,
			ReleaseMetadataRef: metadataRef,
		}},
	}
	writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataDirectoryObjectKey, directory)
	return ws
}

func newReleaseMetadataRoutineTestController(
	t *testing.T,
	le *logrus.Entry,
	b *inmem.Bus,
	stagingDir string,
) *Controller {
	// Construct a launcher with the fixture release pins.
	t.Helper()
	ctrl := &Controller{
		le:          le,
		bus:         b,
		distPeerIDs: releaseAuthorizationTestPins(t),
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:  "spacewave",
					Rev:        1,
					ChannelKey: "stable",
				},
			},
		),
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
	}

	// Configure the release routine retry lifetime.
	ctrl.releaseMetadataRoutine = routine.NewRoutineContainer(
		routine.WithRetry(&backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
			Exponential: &backoff.Exponential{
				InitialInterval: 5,
				MaxInterval:     10,
			},
		}),
	)
	ctrl.releaseMetadataRoutine.SetRoutine(ctrl.refreshCurrentReleaseMetadataStatus)
	return ctrl
}

func buildReleaseMetadataStageUpdateFixture(
	t *testing.T,
	platformID string,
) (
	context.Context,
	*Controller,
	*spacewave_release.ReleaseMetadata,
	*bldr_manifest.ManifestRef,
	*bldr_manifest.ManifestRef,
	string,
) {
	// Build native and companion executable roots.
	t.Helper()
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", platformID)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	manifestRef := writeReleaseManifestTestBlockWithMeta(
		t,
		ctx,
		ws,
		"release/manifests/native",
		src,
		nativeEntrypointManifestID,
		platformID,
		1,
	)

	// Build the required companion manifest.
	cliManifestRef := writeReleaseManifestTestBlockWithBinary(
		t,
		ctx,
		ws,
		"release/manifests/cli",
		cliEntrypointManifestID,
		platformID,
		2,
		"cli",
	)

	// Mount the release fixture and construct its launcher.
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(rel)

	// Allocate staging and return the signed selection set.
	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(t, le, b, stagingDir)
	metadata := testReleaseMetadata("stable", platformID, manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef, cliManifestRef}
	return ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir
}

func waitForUpdatePhase(
	t *testing.T,
	ctrl *Controller,
	phase spacewave_launcher.UpdatePhase,
) *spacewave_launcher.UpdateState {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		info := ctrl.launcherInfoCtr.GetValue()
		state := info.GetUpdateState()
		if state.GetPhase() == phase {
			return state
		}
		if _, err := ctrl.launcherInfoCtr.WaitValueChange(ctx, info, nil); err != nil {
			t.Fatalf("wait for update phase %v: %v, last state=%+v", phase, err, state)
		}
	}
}

// releaseWorldRefreshTestController counts refreshes of the release World.
type releaseWorldRefreshTestController struct {
	calls atomic.Int32
	// refreshed, when set, receives a signal for each refresh without blocking.
	refreshed chan struct{}
}

func (c *releaseWorldRefreshTestController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("release-world-refresh-test", controller.MustParseVersion("0.0.1"), "release world refresh test")
}

func (c *releaseWorldRefreshTestController) Execute(context.Context) error { return nil }

func (c *releaseWorldRefreshTestController) Close() error { return nil }

func (c *releaseWorldRefreshTestController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(bifrost_rpc.LookupRpcService)
	if !ok || dir.LookupRpcServiceID() != cdn_world_controller.WorldRefreshServiceID(releaseWorldEngineID) {
		return nil, nil
	}
	return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
}

func (c *releaseWorldRefreshTestController) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	handler := cdn_world_controller.NewSRPCWorldRefreshHandler(c, cdn_world_controller.WorldRefreshServiceID(releaseWorldEngineID))
	return handler.InvokeMethod(serviceID, methodID, stream)
}

func (c *releaseWorldRefreshTestController) Refresh(context.Context, *cdn_world_controller.RefreshRequest) (*cdn_world_controller.RefreshResponse, error) {
	c.calls.Add(1)
	select {
	case c.refreshed <- struct{}{}:
	default:
	}
	return &cdn_world_controller.RefreshResponse{Accepted: true}, nil
}

type releaseWorldLookupTestController struct {
	ws world.WorldState
}

func (c *releaseWorldLookupTestController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("release-world-test", controller.MustParseVersion("0.0.1"), "release world test")
}

func (c *releaseWorldLookupTestController) Execute(context.Context) error { return nil }

func (c *releaseWorldLookupTestController) Close() error { return nil }

func (c *releaseWorldLookupTestController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(world.LookupWorldEngine)
	if !ok || dir.LookupWorldEngineID() != releaseWorldEngineID {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver[world.LookupWorldEngineValue]([]world.LookupWorldEngineValue{
		&releaseWorldTestEngine{WorldState: c.ws},
	}), nil)
}

type releaseWorldTestEngine struct {
	world.WorldState
}

// StageWorldState rejects writes to this read-only release fixture.
func (e *releaseWorldTestEngine) StageWorldState(context.Context) (world.WorldStage, error) {
	return nil, tx.ErrNotWrite
}

// OperationAuthor identifies this unsigned, read-only release fixture.
func (e *releaseWorldTestEngine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return "", "", nil
}

func (e *releaseWorldTestEngine) NewTransaction(context.Context, bool) (world.Tx, error) {
	return &releaseWorldTestTx{WorldState: e.WorldState}, nil
}

// WaitObjectRev rereads the fixture after each revision.
func (e *releaseWorldTestEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return world.WaitObjectRevBySeqno(ctx, e, key, rev, ignoreNotFound)
}

type releaseWorldTestTx struct {
	world.WorldState
}

func (t *releaseWorldTestTx) Commit(context.Context) error { return nil }

func (t *releaseWorldTestTx) Discard() {}

func writeReleaseManifestTestBlock(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	distDir string,
) *bldr_manifest.ManifestRef {
	t.Helper()
	return writeReleaseManifestTestBlockWithMeta(
		t,
		ctx,
		ws,
		objKey,
		distDir,
		nativeEntrypointManifestID,
		nativeTestPlatformID(),
		1,
	)
}

func writeReleaseManifestTestBlockWithBinary(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	manifestID string,
	platformID string,
	rev uint64,
	contents string,
) *bldr_manifest.ManifestRef {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte(contents), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	return writeReleaseManifestTestBlockWithMeta(
		t,
		ctx,
		ws,
		objKey,
		src,
		manifestID,
		platformID,
		rev,
	)
}

func writeReleaseManifestTestBlockWithMeta(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	distDir string,
	manifestID string,
	platformID string,
	rev uint64,
) *bldr_manifest.ManifestRef {
	return writeReleaseManifestTestBlockWithEntrypointMeta(t, ctx, ws, objKey, distDir, manifestID, platformID, rev, "spacewave")
}

// writeReleaseDesktopArtifactTestBlock creates the platform's installed-app
// shape; Darwin verification uses a signed copy under the test directory.
func writeReleaseDesktopArtifactTestBlock(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	manifestID string,
	platformID string,
	rev uint64,
	contents string,
) *bldr_manifest.ManifestRef {
	// Build the platform installed-app directory shape.
	t.Helper()
	src := t.TempDir()
	entrypoint := "spacewave"
	if isDarwinDesktopPlatform(platformID) {
		entrypoint = "Spacewave.app"
		appDir := filepath.Join(src, entrypoint)
		executable := filepath.Join(appDir, "Contents", "MacOS", "spacewave")
		if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
			t.Fatal(err)
		}
		binary, err := os.ReadFile("/usr/bin/true")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(executable, binary, 0o755); err != nil {
			t.Fatal(err)
		}
		plist := []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleExecutable</key><string>spacewave</string><key>CFBundleIdentifier</key><string>us.aperture.spacewave-test</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`)
		if err := os.WriteFile(filepath.Join(appDir, "Contents", "Info.plist"), plist, 0o644); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command("codesign", "--force", "--sign", "-", appDir).CombinedOutput(); err != nil {
			t.Fatalf("sign test app: %v: %s", err, output)
		}
	} else if err := os.WriteFile(filepath.Join(src, entrypoint), []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return writeReleaseManifestTestBlockWithEntrypointMeta(t, ctx, ws, objKey, src, manifestID, platformID, rev, entrypoint)
}

func writeReleaseManifestTestBlockWithEntrypointMeta(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	distDir string,
	manifestID string,
	platformID string,
	rev uint64,
	entrypoint string,
) *bldr_manifest.ManifestRef {
	// Write the complete executable manifest and file roots.
	t.Helper()
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: manifestID,
		BuildType:  "production",
		PlatformId: platformID,
		Rev:        rev,
	}
	objRef, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		return bldr_manifest.CreateManifestWithIoFS(ctx, bcs, bldr_manifest.NewManifest(meta, entrypoint), os.DirFS(distDir), nil, nil)
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Fixtures use the same release authority as their launcher.
	ref := bldr_manifest.NewManifestRef(meta, objRef)
	if err := ref.SignReleaseAuthorization(releaseAuthorizationTestKey(t)); err != nil {
		t.Fatal(err)
	}
	return ref
}

func writeReleaseMetadataTestBlock(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	blk block.Block,
) *block.BlockRef {
	t.Helper()
	objRef, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(blk, true)
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return objRef.GetRootRef()
}

func testReleaseMetadata(channelKey string, platformID string, ref *block.BlockRef) *spacewave_release.ReleaseMetadata {
	return &spacewave_release.ReleaseMetadata{
		ProjectId:  "spacewave",
		Rev:        1,
		Version:    "0.1.0",
		ChannelKey: channelKey,
		ManifestRefs: []*bldr_manifest.ManifestRef{{
			Meta: &bldr_manifest.ManifestMeta{
				ManifestId: nativeEntrypointManifestID,
				BuildType:  "production",
				PlatformId: platformID,
				Rev:        1,
			},
			ManifestRef: &bucket.ObjectRef{RootRef: ref},
		}},
		BrowserShell: &spacewave_release.BrowserShellMetadata{
			Version:           "0.1.0",
			GenerationId:      "gen-1",
			EntrypointPath:    "/b/entrypoint/boot.mjs",
			ServiceWorkerPath: "/b/entrypoint/sw.js",
			SharedWorkerPath:  "/b/entrypoint/shared-worker.js",
			WasmPath:          "/b/entrypoint/spacewave.wasm",
			Assets: []*spacewave_release.BrowserAsset{{
				Path:        "/b/entrypoint/boot.mjs",
				Size:        1,
				Sha256:      testSHA256(),
				ContentType: "text/javascript",
			}},
		},
		MinimumLauncherVersion: "0.1.0",
	}
}

func testManifestRef(manifestID, platformID string, rev uint64) *bldr_manifest.ManifestRef {
	ref := testBlockRef()
	ref.Hash.Hash[0] = byte(rev)
	return &bldr_manifest.ManifestRef{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: manifestID,
			BuildType:  "production",
			PlatformId: platformID,
			Rev:        rev,
		},
		ManifestRef: &bucket.ObjectRef{RootRef: ref},
	}
}

func nativeTestPlatformID() string {
	platformID, err := nativeDesktopPlatformID()
	if err != nil {
		panic(err)
	}
	return platformID
}

func testBlockRef() *block.BlockRef {
	return &block.BlockRef{
		Hash: &hash.Hash{
			HashType: hash.HashType_HashType_SHA256,
			Hash:     testSHA256(),
		},
	}
}

func testSHA256() []byte {
	out := make([]byte, 32)
	out[0] = 1
	return out
}
