package plugin_host_scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestPluginStatusRecordsAndClearsLastError(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_REQUESTED)
	ctrl.recordPluginStatusError("notes", "left", "download plugin manifest", errors.New("copy failed"))

	status := ctrl.GetPluginStatusCtr().GetValue()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(status.Plugins))
	}
	plugin := status.Plugins[0]
	if plugin.GetLastErrorMessage() != "download plugin manifest: copy failed" {
		t.Fatalf("unexpected last error: %q", plugin.GetLastErrorMessage())
	}
	if plugin.GetLastErrorAt() == nil {
		t.Fatal("expected last error timestamp")
	}
	if plugin.GetState() != bldr_plugin.PluginState_PluginState_REQUESTED {
		t.Fatalf("unexpected state after error: %s", plugin.GetState())
	}

	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_REQUESTED)
	status = ctrl.GetPluginStatusCtr().GetValue()
	if status.Plugins[0].GetLastErrorMessage() == "" {
		t.Fatal("expected requested status update to preserve last error")
	}

	ctrl.setPluginStatusClearingError("notes", "left", bldr_plugin.PluginState_PluginState_RUNNING)
	status = ctrl.GetPluginStatusCtr().GetValue()
	plugin = status.Plugins[0]
	if plugin.GetLastErrorMessage() != "" || plugin.GetLastErrorAt() != nil {
		t.Fatalf("expected running status to clear last error: %#v", plugin)
	}
	if !plugin.GetRunning() || plugin.GetState() != bldr_plugin.PluginState_PluginState_RUNNING {
		t.Fatalf("unexpected running state after clear: %#v", plugin)
	}
}

func TestPluginStatusRecordsTerminalWorkerFailureUntilFreshGenerationRuns(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	ctrl.setPluginStatus("spacewave-core", "", bldr_plugin.PluginState_PluginState_REQUESTED)
	ctrl.recordPluginStatusError(
		"spacewave-core",
		"",
		"execute plugin",
		errors.New("web worker terminal failure before becoming ready: fatal wasm exit"),
	)

	status := ctrl.GetPluginStatusCtr().GetValue()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(status.Plugins))
	}
	plugin := status.Plugins[0]
	if got, want := plugin.GetLastErrorMessage(), "execute plugin: web worker terminal failure before becoming ready: fatal wasm exit"; got != want {
		t.Fatalf("unexpected terminal failure status: got %q want %q", got, want)
	}
	if plugin.GetRunning() {
		t.Fatal("failed generation should not report running")
	}

	ctrl.setPluginStatusClearingError("spacewave-core", "", bldr_plugin.PluginState_PluginState_RUNNING)
	status = ctrl.GetPluginStatusCtr().GetValue()
	plugin = status.Plugins[0]
	if plugin.GetLastErrorMessage() != "" {
		t.Fatalf("fresh running generation should clear terminal failure, got %q", plugin.GetLastErrorMessage())
	}
	if !plugin.GetRunning() {
		t.Fatal("fresh generation should report running")
	}
}

func TestIsPluginRunning(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_REQUESTED)
	if ctrl.IsPluginRunning("notes") {
		t.Fatal("requested plugin should not report running")
	}

	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_RUNNING)
	if !ctrl.IsPluginRunning("notes") {
		t.Fatal("running plugin should report running")
	}
}

func TestWaitPluginsRunningReturnsWhenRequiredPluginsRun(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	ctrl.setPluginStatusClearingError("spacewave-core", "", bldr_plugin.PluginState_PluginState_RUNNING)
	ctrl.setPluginStatusClearingError("spacewave-e2e", "", bldr_plugin.PluginState_PluginState_RUNNING)
	ctrl.setPluginStatus("debug-helper", "", bldr_plugin.PluginState_PluginState_REQUESTED)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ctrl.WaitPluginsRunning(ctx, []string{"spacewave-core", "spacewave-e2e"}); err != nil {
		t.Fatalf("expected required plugins to be running: %v", err)
	}
}

func TestWaitPluginsRunningReturnsRecordedStartupError(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	ctrl.setPluginStatus("spacewave-e2e", "", bldr_plugin.PluginState_PluginState_REQUESTED)
	ctrl.recordPluginStatusError("spacewave-e2e", "", "fetch plugin manifest", errors.New("vite failed"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := ctrl.WaitPluginsRunning(ctx, []string{"spacewave-e2e"})
	if err == nil {
		t.Fatal("expected startup plugin status error")
	}
	if !strings.Contains(err.Error(), "plugin spacewave-e2e failed: fetch plugin manifest: vite failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginStatusSnapshotEqualIncludesLastError(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	ctrl.setPluginStatus("notes", "", bldr_plugin.PluginState_PluginState_REQUESTED)
	before := ctrl.GetPluginStatusCtr().GetValue()
	ctrl.recordPluginStatusError("notes", "", "execute plugin", errors.New("boom"))
	after := ctrl.GetPluginStatusCtr().GetValue()

	if pluginStatusSnapshotEqual(before, after) {
		t.Fatal("expected snapshots with different last errors to differ")
	}
}

func TestPluginStatusUpdateToleratesUninitializedController(t *testing.T) {
	ctrl := &Controller{}

	ctrl.recordPluginStatusError("notes", "", "startup manifest refs", errors.New("skipped"))

	if len(ctrl.pluginStatus) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(ctrl.pluginStatus))
	}
	status := ctrl.buildPluginStatusSnapshotLocked()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin in snapshot, got %d", len(status.Plugins))
	}
	if status.Plugins[0].GetLastErrorMessage() != "startup manifest refs: skipped" {
		t.Fatalf("unexpected last error: %q", status.Plugins[0].GetLastErrorMessage())
	}
}

func TestPluginManifestRecoveryStatusReportsSelectionAndRetainedCandidates(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus:                 make(map[string]*bldr_plugin.PluginStatus),
		pluginManifestRecoveryStatus: make(map[string]*PluginManifestRecoveryStatus),
	}
	executeRef := testObjectRef(t, "execute")
	downloadRef := testObjectRef(t, "download")

	ctrl.recordPluginManifestRecoveryStatus(
		"spacewave-app",
		"",
		&bldr_manifest.ManifestSnapshot{ManifestRef: executeRef},
		&bldr_manifest.ManifestSnapshot{ManifestRef: downloadRef},
		[]*bldr_manifest_world.StartupManifestCandidateEligibility{
			{
				ObjectKey:   "ignored-ref",
				Eligibility: bldr_manifest_world.StartupManifestEligibilityIgnored,
				Reason:      "intermediate:bundle",
			},
			{
				ObjectKey:   "quarantined-ref",
				Eligibility: bldr_manifest_world.StartupManifestEligibilityQuarantined,
				Reason:      "manifest-id-mismatch",
			},
			{
				ObjectKey:   "unsafe-ref",
				Eligibility: bldr_manifest_world.StartupManifestEligibilityUnsafe,
				Reason:      "manifest-read:not-found",
			},
		},
	)

	status := ctrl.GetPluginStatusCtr().GetValue()
	if len(status.ManifestRecovery) != 1 {
		t.Fatalf("recovery rows = %d, want 1", len(status.ManifestRecovery))
	}
	row := status.ManifestRecovery[0]
	if row.ExecuteManifestRef != executeRef.MarshalB58() {
		t.Fatalf("execute ref = %q, want %q", row.ExecuteManifestRef, executeRef.MarshalB58())
	}
	if row.DownloadManifestRef != downloadRef.MarshalB58() {
		t.Fatalf("download ref = %q, want %q", row.DownloadManifestRef, downloadRef.MarshalB58())
	}
	if row.SkippedCandidateCount != 2 {
		t.Fatalf("skipped count = %d, want 2", row.SkippedCandidateCount)
	}
	if row.IgnoredCandidateCount != 1 || !strings.Contains(row.IgnoredCandidateSummary, "ignored-ref") {
		t.Fatalf("unexpected ignored summary: %#v", row)
	}
	if row.QuarantinedCandidateCount != 1 || !strings.Contains(row.QuarantinedCandidateSummary, "quarantined-ref") {
		t.Fatalf("unexpected quarantined summary: %#v", row)
	}

	before := status
	ctrl.recordPluginManifestRecoveryStatus("spacewave-app", "", nil, nil, nil)
	after := ctrl.GetPluginStatusCtr().GetValue()
	if pluginStatusSnapshotEqual(before, after) {
		t.Fatal("expected recovery status changes to change the snapshot")
	}
}

func TestPluginManifestRecoveryStatusClearsWithPluginInstance(t *testing.T) {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus:                 make(map[string]*bldr_plugin.PluginStatus),
		pluginManifestRecoveryStatus: make(map[string]*PluginManifestRecoveryStatus),
	}
	ctrl.updatePluginStatus(
		"spacewave-app",
		"",
		bldr_plugin.PluginState_PluginState_RUNNING,
		"",
		nil,
		false,
		false,
	)
	ctrl.recordPluginManifestRecoveryStatus("spacewave-app", "", nil, nil, nil)
	if len(ctrl.GetPluginStatusCtr().GetValue().ManifestRecovery) != 1 {
		t.Fatalf("expected recovery row before cleanup: %#v", ctrl.GetPluginStatusCtr().GetValue())
	}

	ctrl.updatePluginStatus(
		"spacewave-app",
		"",
		bldr_plugin.PluginState_PluginState_UNKNOWN,
		"",
		nil,
		false,
		false,
	)
	status := ctrl.GetPluginStatusCtr().GetValue()
	if len(status.ManifestRecovery) != 0 {
		t.Fatalf("expected recovery rows cleared with plugin instance, got %#v", status.ManifestRecovery)
	}
}

func testObjectRef(t *testing.T, seed string) *bucket.ObjectRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(seed), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return &bucket.ObjectRef{
		BucketId: "bucket-" + seed,
		RootRef:  ref,
	}
}
