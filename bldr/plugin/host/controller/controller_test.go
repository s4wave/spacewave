package plugin_host_controller

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_logs "github.com/s4wave/spacewave/bldr/plugin/host/logs"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

type testPluginHost struct{}

func (h *testPluginHost) GetPlatformId() string {
	return "test"
}

func (h *testPluginHost) Execute(ctx context.Context) error {
	return nil
}

func (h *testPluginHost) ListPlugins(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (h *testPluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID string,
	instanceKey string,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	return nil
}

func (h *testPluginHost) DeletePlugin(ctx context.Context, pluginID string) error {
	return nil
}

func TestControllerOwnsProcessLifetimeHostRoot(t *testing.T) {
	ctrl := NewController(
		logrus.NewEntry(logrus.New()),
		nil,
		controller.NewInfo("test", controller.MustParseVersion("0.0.1"), "test"),
		&testPluginHost{},
	)
	defer func() {
		if err := ctrl.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()
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
	ctx := t.Context()

	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	ctrlA := NewController(
		le,
		b,
		controller.NewInfo("test-a", controller.MustParseVersion("0.0.1"), "test"),
		&testPluginHost{},
	)
	if got := len(log.Hooks[logrus.WarnLevel]); got != 1 {
		t.Fatalf("host logrus hooks after first controller = %d, want 1", got)
	}

	ctrlB := NewController(
		le,
		b,
		controller.NewInfo("test-b", controller.MustParseVersion("0.0.1"), "test"),
		&testPluginHost{},
	)
	if got := len(log.Hooks[logrus.WarnLevel]); got != 1 {
		t.Fatalf("host logrus hooks after second controller = %d, want 1", got)
	}

	viewA := ctrlA.GetHostRoot().GetStructuredLogs().OpenView(nil, nil)
	defer viewA.Release()
	viewB := ctrlB.GetHostRoot().GetStructuredLogs().OpenView(nil, nil)
	defer viewB.Release()

	le.WithFields(logrus.Fields{
		"plugin-id":    "runner",
		"instance-key": "main",
		"attempt":      2,
	}).Warn("host captured")

	assertCapturedHostLogEvent(t, ctrlA.GetHostRoot().GetStructuredLogs().Snapshot(nil, nil))
	assertCapturedHostLogEvent(t, ctrlB.GetHostRoot().GetStructuredLogs().Snapshot(nil, nil))

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
	ctx := t.Context()

	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	ctrl := NewController(
		le,
		b,
		controller.NewInfo("test", controller.MustParseVersion("0.0.1"), "test"),
		&testPluginHost{},
	)
	defer func() {
		if err := ctrl.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

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

	le.Info("not retained after view release")
	reopened := hub.OpenView(nil, nil)
	defer reopened.Release()
	if got := len(reopened.Snapshot().GetEvents()); got != 0 {
		t.Fatalf("retained events after reopening = %d, want 0", got)
	}
}

func assertCapturedHostLogEvent(t *testing.T, state *plugin_host_logs.StructuredLogState) {
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

// _ is a type assertion
var _ bldr_plugin_host.PluginHost = ((*testPluginHost)(nil))
