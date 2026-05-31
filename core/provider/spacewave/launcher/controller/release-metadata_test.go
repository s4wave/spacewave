//go:build !js

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path/filepath"
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
	want := "release metadata does not support platform " + nativeTestPlatformID()
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
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
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
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: "spacewave-launcher",
		BuildType:  "production",
		PlatformId: nativeTestPlatformID(),
		Rev:        1,
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
				ManifestId: "spacewave-launcher",
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
