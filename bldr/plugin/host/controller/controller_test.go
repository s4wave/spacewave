package plugin_host_controller

import (
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	plugin_host_logs "github.com/s4wave/spacewave/bldr/plugin/host/logs"
	plugin_host_mock "github.com/s4wave/spacewave/bldr/plugin/host/mock"
	"github.com/sirupsen/logrus"
)

func TestControllerOwnsProcessLifetimeHostRoot(t *testing.T) {
	// Construct the plugin-host controller and arrange its cleanup.
	ctrl := NewController(
		logrus.NewEntry(logrus.New()),
		nil,
		controller.NewInfo("test", controller.MustParseVersion("0.0.1"), "test"),
		plugin_host_mock.NewHost("test"),
	)
	defer func() {
		if err := ctrl.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	// Verify the host root provides its desktop tray and structured log registries.
	root := ctrl.GetHostRoot()
	if root == nil {
		t.Fatal("expected host root")
	}
	if root.GetDesktopTray() == nil {
		t.Fatal("expected desktop tray registry")
	}
	if root.GetStructuredLogs() == nil {
		t.Fatal("expected structured log hub")
	}

	// Verify the host-root mux exposes the desktop tray resource service.
	mux := root.GetMux()
	query, ok := mux.(srpc.QueryableInvoker)
	if !ok {
		t.Fatal("expected queryable root mux")
	}
	if !query.HasServiceMethod(
		desktop_tray.SRPCDesktopTrayResourceServiceServiceID,
		"RegisterDesktopTrayEntry",
	) {
		t.Fatal("expected desktop tray resource service on host root")
	}
}

func TestControllerAttachesOneHostLogrusHookPerBus(t *testing.T) {
	// Create a shared controller bus with host logging enabled.
	ctx := t.Context()
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Create the first host controller and verify one logging hook is installed.
	ctrlA := NewController(
		le,
		b,
		controller.NewInfo("test-a", controller.MustParseVersion("0.0.1"), "test"),
		plugin_host_mock.NewHost("test"),
	)
	if got := len(log.Hooks[logrus.WarnLevel]); got != 1 {
		t.Fatalf("host logrus hooks after first controller = %d, want 1", got)
	}

	// Create a second host controller and verify the bus still has one hook.
	ctrlB := NewController(
		le,
		b,
		controller.NewInfo("test-b", controller.MustParseVersion("0.0.1"), "test"),
		plugin_host_mock.NewHost("test"),
	)
	if got := len(log.Hooks[logrus.WarnLevel]); got != 1 {
		t.Fatalf("host logrus hooks after second controller = %d, want 1", got)
	}

	// Open log views on both host roots to retain the shared event.
	viewA := ctrlA.GetHostRoot().GetStructuredLogs().OpenView(nil, nil)
	defer viewA.Release()
	viewB := ctrlB.GetHostRoot().GetStructuredLogs().OpenView(nil, nil)
	defer viewB.Release()

	// Emit a structured host warning with plugin and instance fields.
	le.WithFields(logrus.Fields{
		"plugin-id":    "runner",
		"instance-key": "main",
		"attempt":      2,
	}).Warn("host captured")

	// Verify both host roots capture the shared warning event.
	assertCapturedHostLogEvent(t, ctrlA.GetHostRoot().GetStructuredLogs().Snapshot(nil, nil))
	assertCapturedHostLogEvent(t, ctrlB.GetHostRoot().GetStructuredLogs().Snapshot(nil, nil))

	// Close both host controllers and verify the final close removes the hook.
	if err := ctrlB.Close(); err != nil {
		t.Fatalf("Close ctrlB: %v", err)
	}
	if got := len(log.Hooks[logrus.WarnLevel]); got != 1 {
		t.Fatalf("host logrus hooks after releasing second controller = %d, want 1", got)
	}
	if err := ctrlA.Close(); err != nil {
		t.Fatalf("Close ctrlA: %v", err)
	}
	if got := len(log.Hooks[logrus.WarnLevel]); got != 0 {
		t.Fatalf("host logrus hooks after releasing all controllers = %d, want 0", got)
	}
}

func TestControllerHostLogrusHookDoesNotRetainHistoryAfterViewRelease(t *testing.T) {
	// Create a controller bus for log-history retention checks.
	ctx := t.Context()
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Construct the host controller and arrange its cleanup.
	ctrl := NewController(
		le,
		b,
		controller.NewInfo("test", controller.MustParseVersion("0.0.1"), "test"),
		plugin_host_mock.NewHost("test"),
	)
	defer func() {
		if err := ctrl.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	// Verify releasing the last log view drops the retained history.
	hub := ctrl.GetHostRoot().GetStructuredLogs()
	view := hub.OpenView(nil, nil)
	defer view.Release()
	le.Info("retained while view is open")
	if got := len(hub.Snapshot(nil, nil).GetEvents()); got != 1 {
		t.Fatalf("retained events with open view = %d, want 1", got)
	}
	view.Release()
	if got := len(hub.Snapshot(nil, nil).GetEvents()); got != 0 {
		t.Fatalf("retained events after view release = %d, want 0", got)
	}

	// Verify events emitted without a log view are absent after reopening.
	le.Info("not retained after view release")
	reopened := hub.OpenView(nil, nil)
	defer reopened.Release()
	if got := len(reopened.Snapshot().GetEvents()); got != 0 {
		t.Fatalf("retained events after reopening = %d, want 0", got)
	}
}

// assertCapturedHostLogEvent checks that state captured exactly the one host
// log event the test emitted.
func assertCapturedHostLogEvent(t *testing.T, state *plugin_host_logs.StructuredLogState) {
	// Verify the captured host state contains one event from the expected plugin.
	t.Helper()
	events := state.GetEvents()
	if len(events) != 1 {
		t.Fatalf("captured events = %d, want 1", len(events))
	}
	event := events[0]
	if event.GetPluginId() != "runner" {
		t.Fatalf("plugin id = %q, want runner", event.GetPluginId())
	}
	if event.GetInstanceKey() != "main" {
		t.Fatalf("instance key = %q, want main", event.GetInstanceKey())
	}

	// Verify the captured event preserves its stream, severity, message and fields.
	if event.GetStream() != plugin_host_logs.StructuredLogStream_STRUCTURED_LOG_STREAM_LOGGER {
		t.Fatalf("stream = %s, want logger", event.GetStream())
	}
	if event.GetLevel() != plugin_host_logs.StructuredLogLevel_STRUCTURED_LOG_LEVEL_WARN {
		t.Fatalf("level = %s, want warn", event.GetLevel())
	}
	if event.GetMessage() != "host captured" {
		t.Fatalf("message = %q, want host captured", event.GetMessage())
	}
	if event.GetFields()["attempt"] != "2" {
		t.Fatalf("attempt field = %q, want 2", event.GetFields()["attempt"])
	}
}
