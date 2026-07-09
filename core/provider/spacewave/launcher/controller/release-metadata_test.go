//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	controller_info "github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	spacewave_release "github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

func TestReadSelectedReleaseMetadata(t *testing.T) {
	ctx := context.Background()
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())

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

func TestReadSelectedReleaseMetadataErrors(t *testing.T) {
	ctx := context.Background()
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", "desktop/other/arch")

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

	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{testManifestRef("spacewave-plugin", platformID, 1)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "non-entrypoint native manifests") {
		t.Fatalf("wrong-identity error = %v", err)
	}

	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{valid, testManifestRef(nativeEntrypointManifestID, platformID, 2)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "duplicate native entrypoint manifest") {
		t.Fatalf("duplicate error = %v", err)
	}

	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{testManifestRef(nativeEntrypointManifestID, "desktop/other/arch", 1)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "missing native entrypoint manifest") {
		t.Fatalf("missing error = %v", err)
	}
}

func TestSelectCLIReleaseManifestRefRequiresCLIEntrypointIdentity(t *testing.T) {
	platformID := nativeTestPlatformID()
	desktop := testManifestRef(nativeEntrypointManifestID, platformID, 1)
	cli := testManifestRef(cliEntrypointManifestID, platformID, 2)
	selected, err := selectCLIReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{
			testManifestRef("spacewave-plugin", platformID, 1),
			desktop,
			cli,
		},
	}, platformID)
	if err != nil {
		t.Fatalf("selectCLIReleaseManifestRef() error = %v", err)
	}
	if selected != cli {
		t.Fatal("selector did not return the cli entrypoint ref")
	}

	if _, err := selectCLIReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{desktop},
	}, platformID); err == nil || !strings.Contains(err.Error(), "missing spacewave-cli") {
		t.Fatalf("wrong-identity error = %v", err)
	}

	if _, err := selectCLIReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{cli, testManifestRef(cliEntrypointManifestID, platformID, 3)},
	}, platformID); err == nil || !strings.Contains(err.Error(), "duplicate cli entrypoint manifest") {
		t.Fatalf("duplicate error = %v", err)
	}

	if _, err := selectReleaseManifestRef(&spacewave_release.ReleaseMetadata{
		ManifestRefs: []*bldr_manifest.ManifestRef{cli},
	}, platformID); err == nil || !strings.Contains(err.Error(), "missing spacewave-dist") {
		t.Fatalf("cli should not satisfy native selector: %v", err)
	}
}

func TestCheckoutReleaseManifestStagesDist(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err.Error())
	}
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
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err.Error())
	}
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

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	stagingDir := t.TempDir()
	ctrl := &Controller{
		le:  le,
		bus: b,
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:  "spacewave",
					Rev:        1,
					ChannelKey: "stable",
				},
			},
		),
		fetchStatusCtr: ccontainer.NewCContainer[*spacewave_launcher.FetchStatus](
			&spacewave_launcher.FetchStatus{},
		),
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
	}
	ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig())

	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_STAGED {
		t.Fatalf("phase = %v error=%q", state.GetPhase(), state.GetErrorMessage())
	}
	if state.GetStagedPath() != filepath.Join(stagingDir, "0.1.0", "dist", "spacewave") {
		t.Fatalf("staged path = %q", state.GetStagedPath())
	}
	got, err := os.ReadFile(state.GetStagedPath())
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(got) != "binary" {
		t.Fatalf("staged binary = %q", string(got))
	}
	if outcome := ctrl.fetchStatusCtr.GetValue().ReleaseMetadataOutcome; outcome != "staged" {
		t.Fatalf("release metadata outcome = %q, want staged", outcome)
	}
	fetchStatus := ctrl.fetchStatusCtr.GetValue()
	if fetchStatus.SelectedEntrypointManifestID != nativeEntrypointManifestID {
		t.Fatalf("selected entrypoint id = %q", fetchStatus.SelectedEntrypointManifestID)
	}
	if fetchStatus.SelectedEntrypointPlatformID != nativeTestPlatformID() {
		t.Fatalf("selected entrypoint platform = %q", fetchStatus.SelectedEntrypointPlatformID)
	}
	if fetchStatus.SelectedEntrypointManifestRev != 1 {
		t.Fatalf("selected entrypoint rev = %d", fetchStatus.SelectedEntrypointManifestRev)
	}
	if fetchStatus.SelectedEntrypointManifestRef == "" {
		t.Fatal("selected entrypoint ref is empty")
	}
	if fetchStatus.SelectedCLIManifestID != cliEntrypointManifestID {
		t.Fatalf("selected CLI entrypoint id = %q", fetchStatus.SelectedCLIManifestID)
	}
	if fetchStatus.SelectedCLIPlatformID != nativeTestPlatformID() {
		t.Fatalf("selected CLI entrypoint platform = %q", fetchStatus.SelectedCLIPlatformID)
	}
	if fetchStatus.SelectedCLIManifestRev != 2 {
		t.Fatalf("selected CLI entrypoint rev = %d", fetchStatus.SelectedCLIManifestRev)
	}
	if fetchStatus.SelectedCLIManifestRef == "" {
		t.Fatal("selected CLI entrypoint ref is empty")
	}
	if fetchStatus.SelectedCLIBinaryPath != filepath.Join(stagingDir, "0.1.0", "cli-dist", "spacewave") {
		t.Fatalf("selected CLI binary path = %q", fetchStatus.SelectedCLIBinaryPath)
	}
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
	cliBinary, err := os.ReadFile(fetchStatus.SelectedCLIBinaryPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(cliBinary) != "cli" {
		t.Fatalf("staged CLI binary = %q", string(cliBinary))
	}
	if fetchStatus.ReleaseWorldHeadRef == "" {
		t.Fatal("release world head ref is empty")
	}
}

func TestRefreshReleaseMetadataStatusClearsStaleReleaseWorldHeadOnError(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	ctrl := &Controller{
		le:  le,
		bus: b,
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{},
		),
		fetchStatusCtr: ccontainer.NewCContainer[*spacewave_launcher.FetchStatus](
			&spacewave_launcher.FetchStatus{
				ReleaseWorldHeadRef:           "previous-head",
				SelectedEntrypointManifestRef: "previous-manifest",
				SelectedCLIManifestRef:        "previous-cli-manifest",
			},
		),
		stagingDirFunc: func() (string, error) { return t.TempDir(), nil },
	}
	err = ctrl.refreshReleaseMetadataStatus(ctx, &spacewave_launcher.DistConfig{
		ProjectId:  "spacewave",
		Rev:        1,
		ChannelKey: "missing",
	})
	if err == nil {
		t.Fatal("expected missing channel error")
	}
	fetchStatus := ctrl.fetchStatusCtr.GetValue()
	if fetchStatus.ReleaseMetadataOutcome != "error" {
		t.Fatalf("release metadata outcome = %q, want error", fetchStatus.ReleaseMetadataOutcome)
	}
	if fetchStatus.ReleaseWorldHeadRef != "" {
		t.Fatalf("release world head ref = %q, want cleared", fetchStatus.ReleaseWorldHeadRef)
	}
	if fetchStatus.SelectedEntrypointManifestRef != "" {
		t.Fatalf("selected entrypoint ref = %q, want cleared", fetchStatus.SelectedEntrypointManifestRef)
	}
	if fetchStatus.SelectedCLIManifestRef != "" {
		t.Fatalf("selected CLI entrypoint ref = %q, want cleared", fetchStatus.SelectedCLIManifestRef)
	}
}

func TestRefreshReleaseMetadataStatusRejectsDirectoryEntrypoint(t *testing.T) {
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

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(le, b, stagingDir)
	err = ctrl.refreshReleaseMetadataStatus(ctx, ctrl.launcherInfoCtr.GetValue().GetDistConfig())
	if err == nil {
		t.Fatal("expected directory entrypoint error")
	}
	if !strings.Contains(err.Error(), "staged directory entrypoint must be a .app bundle") {
		t.Fatalf("error = %q", err.Error())
	}
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_ERROR {
		t.Fatalf("phase = %v, want ERROR", state.GetPhase())
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "0.1.0")); !os.IsNotExist(err) {
		t.Fatalf("stage root should be removed, stat err = %v", err)
	}
}

func TestReleaseMetadataRoutineRetriesUntilReleaseWorldMounted(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
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
		nativeTestPlatformID(),
		1,
	)
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

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(le, b, stagingDir)
	ctrl.releaseMetadataRoutine.SetContext(ctx, true)
	defer ctrl.releaseMetadataRoutine.ClearContext()

	waitForUpdatePhase(t, ctrl, spacewave_launcher.UpdatePhase_UpdatePhase_ERROR)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	state := waitForUpdatePhase(t, ctrl, spacewave_launcher.UpdatePhase_UpdatePhase_STAGED)
	if state.GetStagedPath() != filepath.Join(stagingDir, "0.1.0", "dist", "spacewave") {
		t.Fatalf("staged path = %q", state.GetStagedPath())
	}
}

func TestRefreshReleaseMetadataStatusErrorsWhenNativeManifestMissing(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", "js")

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	ctrl := newReleaseMetadataRoutineTestController(le, b, t.TempDir())
	ctrl.launcherInfoCtr.SetValue(&spacewave_launcher.LauncherInfo{
		DistConfig: ctrl.launcherInfoCtr.GetValue().GetDistConfig(),
		UpdateState: &spacewave_launcher.UpdateState{
			Phase:        spacewave_launcher.UpdatePhase_UpdatePhase_ERROR,
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
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_ERROR {
		t.Fatalf("phase = %v, want ERROR", state.GetPhase())
	}
	if !strings.Contains(state.GetErrorMessage(), want) {
		t.Fatalf("error message = %q, want %q", state.GetErrorMessage(), want)
	}
	if outcome := ctrl.fetchStatusCtr.GetValue().ReleaseMetadataOutcome; outcome != "error" {
		t.Fatalf("release metadata outcome = %q, want error", outcome)
	}
}

func TestRefreshReleaseMetadataStatusErrorsWhenCLIManifestMissing(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := buildReleaseMetadataTestWorld(t, ctx, "stable", nativeTestPlatformID())
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	manifestRef := writeReleaseManifestTestBlock(t, ctx, ws, "release/manifests/native", src)
	metadata := testReleaseMetadata("stable", nativeTestPlatformID(), manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef}
	metadataRef := writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataObjectKey("stable"), metadata)
	writeReleaseMetadataTestBlock(t, ctx, ws, releaseMetadataDirectoryObjectKey, &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{
			ChannelKey:         "stable",
			ReleaseMetadataRef: metadataRef,
		}},
	})

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	ctrl := newReleaseMetadataRoutineTestController(le, b, t.TempDir())
	err = ctrl.refreshCurrentReleaseMetadataStatus(ctx)
	if err == nil {
		t.Fatal("expected missing CLI manifest error")
	}
	want := "missing " + cliEntrypointManifestID
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if outcome := ctrl.fetchStatusCtr.GetValue().ReleaseMetadataOutcome; outcome != "error" {
		t.Fatalf("release metadata outcome = %q, want error", outcome)
	}
}

func TestStageReleaseManifestUpdateRejectsRawDarwinInstalledAppPayload(t *testing.T) {
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

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(le, b, stagingDir)
	ctrl.currentExecutableBundleFunc = func() (string, bool, string, error) {
		return filepath.Join(t.TempDir(), "Spacewave.app", "Contents", "MacOS", "spacewave"), true, "/Applications/Spacewave.app", nil
	}
	metadata := testReleaseMetadata("stable", "desktop/darwin/arm64", manifestRef.GetManifestRef().GetRootRef())
	metadata.ManifestRefs = []*bldr_manifest.ManifestRef{manifestRef, cliManifestRef}
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
	ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
	metadata.Version = "../escape"

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
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows hosts")
	}
	ctx, ctrl, metadata, manifestRef, cliManifestRef, stagingDir := buildReleaseMetadataStageUpdateFixture(t, nativeTestPlatformID())
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(stagingDir, "0.1.0")); err != nil {
		t.Fatal(err.Error())
	}

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

func TestVerifyStagedCLIEntrypointRejectsSymlink(t *testing.T) {
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
	stagedPath := filepath.Join(cliDistPath, "spacewave")
	if err := os.Symlink(outside, stagedPath); err != nil {
		t.Fatal(err.Error())
	}
	err := verifyStagedCLIEntrypoint(stageRoot, cliDistPath, stagedPath)
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
	eng, err := world_block.NewEngine(ctx, le, ocs, nil, nil, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Fatal(err.Error())
		}
	})
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
	le *logrus.Entry,
	b *inmem.Bus,
	stagingDir string,
) *Controller {
	ctrl := &Controller{
		le:  le,
		bus: b,
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:  "spacewave",
					Rev:        1,
					ChannelKey: "stable",
				},
			},
		),
		fetchStatusCtr: ccontainer.NewCContainer[*spacewave_launcher.FetchStatus](
			&spacewave_launcher.FetchStatus{},
		),
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
	}
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
	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	rel, err := b.AddController(ctx, &releaseWorldLookupTestController{ws: ws}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(rel)
	stagingDir := t.TempDir()
	ctrl := newReleaseMetadataRoutineTestController(le, b, stagingDir)
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

type releaseWorldLookupTestController struct {
	ws world.WorldState
}

func (c *releaseWorldLookupTestController) GetControllerInfo() *controller_info.Info {
	return controller_info.NewInfo("release-world-test", controller.MustParseVersion("0.0.1"), "release world test")
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

func (e *releaseWorldTestEngine) NewTransaction(context.Context, bool) (world.Tx, error) {
	return &releaseWorldTestTx{WorldState: e.WorldState}, nil
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
	t.Helper()
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: manifestID,
		BuildType:  "production",
		PlatformId: platformID,
		Rev:        rev,
	}
	objRef, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		_, err := bldr_manifest.CreateManifestWithIoFS(ctx, bcs, meta, "spacewave", os.DirFS(distDir), nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return bldr_manifest.NewManifestRef(meta, objRef)
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
