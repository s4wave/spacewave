package plugin_host_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
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
		controller.NewInfo("test", semver.MustParse("0.0.1"), "test"),
		&testPluginHost{},
	)
	root := ctrl.GetHostRoot()
	if root == nil {
		t.Fatal("expected host root")
	}
	if root.GetDesktopTray() == nil {
		t.Fatal("expected desktop tray registry")
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

// _ is a type assertion
var _ bldr_plugin_host.PluginHost = ((*testPluginHost)(nil))
