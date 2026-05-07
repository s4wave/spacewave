package electron

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_resource "github.com/s4wave/spacewave/bldr/plugin/host/resource"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

func TestOpenPluginHostDesktopTrayUsesPluginHostResourceBoundary(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	hostRoot := plugin_host_root.NewRoot()
	pluginRoot := plugin_host_resource.NewPluginHostRoot(
		le,
		b,
		"web",
		"main",
		nil,
		nil,
		nil,
		hostRoot,
		"state-atoms",
		bldr_plugin.PluginVolumeID,
	)
	defer pluginRoot.Release()

	hostMux := srpc.NewMux()
	resourceServer := resource_server.NewResourceServer(pluginRoot.GetMux())
	if err := resourceServer.Register(hostMux); err != nil {
		t.Fatal(err)
	}
	hostClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(hostMux)))
	rpcCtrl := bifrost_rpc.NewClientController(
		le,
		b,
		controller.NewInfo("test/plugin-host-resource-client", semver.MustParse("0.0.1"), ""),
		hostClient,
		[]string{bldr_plugin.HostServiceIDPrefix},
	)
	rel, err := b.AddController(ctx, rpcCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	hostTray := desktop_tray.NewSRPCDesktopTrayResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(hostRoot.GetMux()))),
	)
	strm, err := hostTray.WatchDesktopTray(ctx, &desktop_tray.WatchDesktopTrayRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if state := recvDesktopTrayState(t, strm); len(state.GetEntries()) != 0 {
		t.Fatalf("initial host tray entries = %d, want 0", len(state.GetEntries()))
	}

	source, err := openPluginHostDesktopTray(ctx, b)
	if err != nil {
		t.Fatal(err)
	}

	_, err = source.tray.RegisterDesktopTrayEntry(ctx, &desktop_tray.RegisterDesktopTrayEntryRequest{
		Entry: &desktop_tray.DesktopTrayEntry{
			Id:      "status",
			Kind:    desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label:   "Runtime - Running",
			Enabled: false,
		},
	})
	if err != nil {
		source.Release()
		t.Fatal(err)
	}

	state := recvDesktopTrayState(t, strm)
	if len(state.GetEntries()) != 1 || state.GetEntries()[0].GetLabel() != "Runtime - Running" {
		source.Release()
		t.Fatalf("host tray entries after plugin-host registration = %#v", state.GetEntries())
	}

	source.Release()
	state = recvDesktopTrayState(t, strm)
	if len(state.GetEntries()) != 0 {
		t.Fatalf("host tray entries after source release = %d, want 0", len(state.GetEntries()))
	}
}

func recvDesktopTrayState(
	t *testing.T,
	strm desktop_tray.SRPCDesktopTrayResourceService_WatchDesktopTrayClient,
) *desktop_tray.DesktopTrayState {
	t.Helper()
	resp, err := strm.Recv()
	if err != nil {
		t.Fatalf("recv desktop tray state: %v", err)
	}
	return resp.GetState()
}

func TestShouldExitWithoutRestart(t *testing.T) {
	if !shouldExitWithoutRestart(errors.New("stream reset"), nil, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected clean exit + exit policy to stop restart")
	}
	if shouldExitWithoutRestart(errors.New("stream reset"), nil, QuitPolicy_QUIT_POLICY_RESTART) {
		t.Fatal("expected restart policy to keep restart behavior")
	}
	if shouldExitWithoutRestart(nil, exec.ErrNotFound, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected non-zero process exit to keep restart behavior")
	}
	if !shouldExitWithoutRestart(errors.New("stream reset"), context.DeadlineExceeded, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected stream reset + exit policy to stop restart")
	}
	if shouldExitWithoutRestart(errors.New("unexpected disconnect"), context.DeadlineExceeded, QuitPolicy_QUIT_POLICY_EXIT) {
		t.Fatal("expected unexpected disconnect to keep restart behavior")
	}
}
