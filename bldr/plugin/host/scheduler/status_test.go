package plugin_host_scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestWaitControllerOnBusWaitsForDelayedScheduler(t *testing.T) {
	// Bind the bus context.
	busCtx := t.Context()

	// Build the bus, a peer, and the scheduler controller.
	logger := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(directive_controller.NewController(busCtx, logger))
	p, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	scheduler := NewController(logger, b, NewConfig(
		"",
		"test-engine",
		"test-plugin-host",
		"test-volume",
		p.GetPeerID().String(),
		true,
		false,
		false,
	))

	// Start the wait in the background before the scheduler registers.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type waitResult struct {
		scheduler *Controller
		err       error
	}
	resultCh := make(chan waitResult, 1)
	go func() {
		got, err := WaitControllerOnBus(ctx, b)
		resultCh <- waitResult{scheduler: got, err: err}
	}()

	// The wait must still be blocked after a short delay.
	delay := time.NewTimer(50 * time.Millisecond)
	defer delay.Stop()
	select {
	case result := <-resultCh:
		t.Fatalf("wait returned before scheduler registration: scheduler=%p err=%v", result.scheduler, result.err)
	case <-delay.C:
	}

	// Register the scheduler controller on the bus.
	rel, err := b.AddController(busCtx, scheduler, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// The wait returns the registered scheduler.
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("wait: %v", result.err)
		}
		if result.scheduler != scheduler {
			t.Fatalf("scheduler = %p, want %p", result.scheduler, scheduler)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not return after scheduler registration")
	}
}

func TestWaitControllerOnBusReturnsContextCancellation(t *testing.T) {
	// Bind the bus context and build the bus.
	busCtx := t.Context()
	logger := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(directive_controller.NewController(busCtx, logger))

	// Start the wait and cancel its context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, err := WaitControllerOnBus(ctx, b)
		resultCh <- err
	}()
	cancel()

	// The wait returns context.Canceled.
	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not return after context cancellation")
	}
}

func TestPluginStatusRecordsAndClearsLastError(t *testing.T) {
	// Build a controller with an equal-compared status snapshot.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	// Record an error on a requested plugin.
	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_REQUESTED)
	ctrl.recordPluginStatusError("notes", "left", "download plugin manifest", errors.New("copy failed"))

	// The snapshot carries the error message, timestamp, and state.
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

	// A plain requested status update preserves the last error.
	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_REQUESTED)
	status = ctrl.GetPluginStatusCtr().GetValue()
	if status.Plugins[0].GetLastErrorMessage() == "" {
		t.Fatal("expected requested status update to preserve last error")
	}

	// The running status clears the last error.
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
	// Build a controller with an equal-compared status snapshot.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	// Record a terminal worker failure on the requested plugin.
	ctrl.setPluginStatus("spacewave-core", "", bldr_plugin.PluginState_PluginState_REQUESTED)
	ctrl.recordPluginStatusError(
		"spacewave-core",
		"",
		"execute plugin",
		errors.New("web worker terminal failure before becoming ready: fatal wasm exit"),
	)

	// The snapshot reports the terminal failure and no running generation.
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

	// A fresh running generation clears the terminal failure.
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
	// Build a controller with an equal-compared status snapshot.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	// A requested plugin does not report running.
	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_REQUESTED)
	if ctrl.IsPluginRunning("notes") {
		t.Fatal("requested plugin should not report running")
	}

	// A running plugin reports running.
	ctrl.setPluginStatus("notes", "left", bldr_plugin.PluginState_PluginState_RUNNING)
	if !ctrl.IsPluginRunning("notes") {
		t.Fatal("running plugin should report running")
	}
}

func TestWaitPluginsRunningReturnsWhenRequiredPluginsRun(t *testing.T) {
	// Build a controller with an equal-compared status snapshot.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	// Mark the required plugins running and one other requested.
	ctrl.setPluginStatusClearingError("spacewave-core", "", bldr_plugin.PluginState_PluginState_RUNNING)
	ctrl.setPluginStatusClearingError("spacewave-e2e", "", bldr_plugin.PluginState_PluginState_RUNNING)
	ctrl.setPluginStatus("debug-helper", "", bldr_plugin.PluginState_PluginState_REQUESTED)

	// Waiting for the required plugins returns immediately.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ctrl.WaitPluginsRunning(ctx, []string{"spacewave-core", "spacewave-e2e"}); err != nil {
		t.Fatalf("expected required plugins to be running: %v", err)
	}
}

func TestWaitPluginsRunningReturnsRecordedStartupError(t *testing.T) {
	// Build a controller with an equal-compared status snapshot.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	// Record a startup error on a requested plugin.
	ctrl.setPluginStatus("spacewave-e2e", "", bldr_plugin.PluginState_PluginState_REQUESTED)
	ctrl.recordPluginStatusError("spacewave-e2e", "", "fetch plugin manifest", errors.New("vite failed"))

	// Waiting returns the recorded startup error.
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
	// Build a controller with an equal-compared status snapshot.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}

	// Snapshot the status before and after recording an error.
	ctrl.setPluginStatus("notes", "", bldr_plugin.PluginState_PluginState_REQUESTED)
	before := ctrl.GetPluginStatusCtr().GetValue()
	ctrl.recordPluginStatusError("notes", "", "execute plugin", errors.New("boom"))
	after := ctrl.GetPluginStatusCtr().GetValue()

	// The snapshots differ because the last error changed.
	if before.EqualVT(after) {
		t.Fatal("expected snapshots with different last errors to differ")
	}
}

func TestPluginStatusUpdateToleratesUninitializedController(t *testing.T) {
	// Build a controller with no status map or container.
	ctrl := &Controller{}

	// Record an error on the uninitialized controller.
	ctrl.recordPluginStatusError("notes", "", "startup manifest refs", errors.New("skipped"))

	// The recorded error is visible in the rebuilt snapshot.
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
	// Build a controller with recovery status tracking and test object refs.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus:                 make(map[string]*bldr_plugin.PluginStatus),
		pluginManifestRecoveryStatus: make(map[string]*bldr_plugin.PluginManifestRecoveryStatus),
	}
	executeRef := testObjectRef(t, "execute")
	downloadRef := testObjectRef(t, "download")

	// Record recovery status with ignored, quarantined, and unsafe candidates.
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

	// The recovery row carries the refs and candidate summaries.
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

	// Clearing the recovery status changes the snapshot.
	before := status
	ctrl.recordPluginManifestRecoveryStatus("spacewave-app", "", nil, nil, nil)
	after := ctrl.GetPluginStatusCtr().GetValue()
	if before.EqualVT(after) {
		t.Fatal("expected recovery status changes to change the snapshot")
	}
}

func TestPluginManifestRecoveryStatusClearsWithPluginInstance(t *testing.T) {
	// Build a controller with recovery status tracking.
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus:                 make(map[string]*bldr_plugin.PluginStatus),
		pluginManifestRecoveryStatus: make(map[string]*bldr_plugin.PluginManifestRecoveryStatus),
	}

	// Record a recovery row for a running plugin instance.
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

	// Removing the plugin instance clears its recovery rows.
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

// testObjectRef builds a deterministic bucket object ref for a seed.
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

func TestResolveLoadPluginScopesUnqualifiedDirectiveToScheduler(t *testing.T) {
	// Build a Space scheduler controller.
	conf := NewConfig("space-a", "engine", "plugin-host", "volume", "peer", true, false, false)
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, conf)

	// An unqualified LoadPlugin directive resolves on the scheduler.
	resolver, err := ctrl.resolveLoadPlugin(bldr_plugin.NewLoadPlugin("notes"))
	if err != nil {
		t.Fatal(err)
	}
	if resolver == nil {
		t.Fatal("unqualified directive did not resolve")
	}
}

// TestResolveLoadPluginResolvesOtherInstances keeps per-object plugin
// instances, such as one V86 runtime per VM, loadable on a Space scheduler.
func TestResolveLoadPluginResolvesOtherInstances(t *testing.T) {
	// Build a Space scheduler controller.
	conf := NewConfig("space-a", "engine", "plugin-host", "volume", "peer", true, false, false)
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, conf)

	// An instanced LoadPlugin directive resolves for another object instance.
	resolver, err := ctrl.resolveLoadPlugin(bldr_plugin.NewLoadPluginInstanced("spacewave-v86", "vm-1"))
	if err != nil {
		t.Fatal(err)
	}
	if resolver == nil {
		t.Fatal("instanced directive did not resolve")
	}
}
