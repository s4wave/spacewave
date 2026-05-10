package plugin_host_scheduler

import (
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
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
