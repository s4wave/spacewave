package electron

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_resource "github.com/s4wave/spacewave/bldr/plugin/host/resource"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	bldr_resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
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
		controller.NewInfo("test/plugin-host-resource-client", controller.MustParseVersion("0.0.1"), ""),
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

func TestDesktopTrayReconcilerPublishesHostTrayToElectronMainWithoutRenderer(t *testing.T) {
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
		controller.NewInfo("test/plugin-host-resource-client", controller.MustParseVersion("0.0.1"), ""),
		hostClient,
		[]string{bldr_plugin.HostServiceIDPrefix},
	)
	rel, err := b.AddController(ctx, rpcCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	source, err := openPluginHostDesktopTray(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Release()
	_, err = source.tray.RegisterDesktopTrayEntry(ctx, &desktop_tray.RegisterDesktopTrayEntryRequest{
		Entry: &desktop_tray.DesktopTrayEntry{
			Id:      "status-runtime",
			Kind:    desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
			Label:   "CLI reachable - no renderer window",
			Enabled: false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	targetTray := desktop_tray.NewDesktopTray()
	targetDirect := desktop_tray.NewSRPCDesktopTrayResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(targetTray.GetMux()))),
	)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	targetStream, err := targetDirect.WatchDesktopTray(watchCtx, &desktop_tray.WatchDesktopTrayRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if state := recvDesktopTrayState(t, targetStream); len(state.GetEntries()) != 0 {
		t.Fatalf("target tray initial entries = %d, want 0", len(state.GetEntries()))
	}

	runtime := newDesktopRuntimeOnlyWebRuntime(t, targetTray)
	ctrl := &Controller{le: le, bus: b}
	reconcileCtx, reconcileCancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.reconcileDesktopTray(reconcileCtx, runtime)
	}()

	state := recvDesktopTrayState(t, targetStream)
	if len(state.GetEntries()) != 1 {
		reconcileCancel()
		t.Fatalf("target tray entries after reconcile = %d, want 1", len(state.GetEntries()))
	}
	if state.GetEntries()[0].GetLabel() != "CLI reachable - no renderer window" {
		reconcileCancel()
		t.Fatalf("target tray label = %q", state.GetEntries()[0].GetLabel())
	}
	if runtime.connectCount.Load() != 1 {
		reconcileCancel()
		t.Fatalf("desktop runtime resource connections = %d, want 1", runtime.connectCount.Load())
	}
	if runtime.createWebDocumentCount.Load() != 0 {
		reconcileCancel()
		t.Fatalf("renderer window creates = %d, want 0", runtime.createWebDocumentCount.Load())
	}

	reconcileCancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected reconciler to stop on context cancel, got %v", err)
	}
}

func TestDesktopTrayReconcilerRetriesSourceFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	var attempts atomic.Int32
	var notified atomic.Bool
	retried := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDesktopTrayReconcilerUntilCanceled(ctx, le, func(ctx context.Context) error {
			if attempts.Add(1) == 1 {
				return errors.New("source stream closed")
			}
			if notified.CompareAndSwap(false, true) {
				close(retried)
			}
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	select {
	case err := <-errCh:
		t.Fatalf("reconciler stopped after source failure: %v", err)
	case <-retried:
	}
	if attempts.Load() != 2 {
		t.Fatalf("reconciler attempts = %d, want 2", attempts.Load())
	}

	cancel()
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("expected reconciler to stop on context cancel, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reconciler cancellation")
	}
}

func TestDesktopTrayMirroredRuntimeDoesNotExitOnTrayFailure(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	runtime := newDesktopRuntimeOnlyWebRuntime(t, desktop_tray.NewDesktopTray())
	trayFailed := make(chan struct{})
	secondAttempt := make(chan struct{})
	cancelObserved := make(chan struct{})
	releaseCancel := make(chan struct{})
	var attempts atomic.Int32
	var secondNotified atomic.Bool
	mirrored := &desktopTrayMirroredRuntime{
		WebRuntime: runtime,
		controller: &Controller{le: le},
		reconcileDesktopTrayRaw: func(ctx context.Context, rt web_runtime.WebRuntime) error {
			if attempts.Add(1) == 1 {
				close(trayFailed)
				return errors.New("source stream closed")
			}
			if secondNotified.CompareAndSwap(false, true) {
				close(secondAttempt)
			}
			<-ctx.Done()
			close(cancelObserved)
			<-releaseCancel
			return ctx.Err()
		},
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- mirrored.Execute(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("mirrored runtime exited after tray failure before parent cancel: %v", err)
	case <-trayFailed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tray reconciler failure")
	}

	select {
	case err := <-errCh:
		t.Fatalf("mirrored runtime exited after tray failure before parent cancel: %v", err)
	case <-secondAttempt:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tray reconciler retry")
	}

	cancel()
	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tray reconciler cancellation")
	}
	select {
	case err := <-errCh:
		t.Fatalf("mirrored runtime exited before tray reconciler cleanup finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCancel)
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("expected mirrored runtime to stop on parent cancel, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for mirrored runtime cancellation")
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

type desktopRuntimeOnlyWebRuntime struct {
	resourceService        bldr_resource.SRPCResourceServiceClient
	statusCtr              *ccontainer.CContainer[*web_runtime.WebRuntimeStatus]
	connectCount           atomic.Int32
	createWebDocumentCount atomic.Int32
}

func newDesktopRuntimeOnlyWebRuntime(
	t *testing.T,
	tray *desktop_tray.DesktopTray,
) *desktopRuntimeOnlyWebRuntime {
	t.Helper()

	server := resource_server.NewResourceServer(tray.GetMux())
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatalf("register desktop runtime resource server: %v", err)
	}
	service := bldr_resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))),
	)
	return &desktopRuntimeOnlyWebRuntime{
		resourceService: service,
		statusCtr:       ccontainer.NewCContainerVT(&web_runtime.WebRuntimeStatus{}),
	}
}

func (r *desktopRuntimeOnlyWebRuntime) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (r *desktopRuntimeOnlyWebRuntime) GetWebRuntimeStatusCtr() *ccontainer.CContainer[*web_runtime.WebRuntimeStatus] {
	return r.statusCtr
}

func (r *desktopRuntimeOnlyWebRuntime) GetWebDocuments(ctx context.Context) (map[string]web_document.WebDocument, error) {
	return map[string]web_document.WebDocument{}, nil
}

func (r *desktopRuntimeOnlyWebRuntime) GetWebDocument(
	ctx context.Context,
	webDocumentID string,
	wait bool,
) (web_document.WebDocument, error) {
	return nil, nil
}

func (r *desktopRuntimeOnlyWebRuntime) GetWebDocumentOpenStream(webDocumentID string) srpc.OpenStreamFunc {
	return unavailableRendererOpenStream
}

func (r *desktopRuntimeOnlyWebRuntime) WaitReady(ctx context.Context) error {
	return nil
}

func (r *desktopRuntimeOnlyWebRuntime) WaitFirstWebDocument(ctx context.Context) (web_document.WebDocument, error) {
	return nil, errors.New("renderer window is unavailable")
}

func (r *desktopRuntimeOnlyWebRuntime) ConnectDesktopRuntimeResourceClient(
	ctx context.Context,
) (*resource_client.Client, error) {
	r.connectCount.Add(1)
	return resource_client.NewClient(ctx, r.resourceService)
}

func (r *desktopRuntimeOnlyWebRuntime) CreateWebDocument(ctx context.Context, webViewID string) (bool, error) {
	r.createWebDocumentCount.Add(1)
	return false, errors.New("renderer window is unavailable")
}

func (r *desktopRuntimeOnlyWebRuntime) FlushIndexCache(ctx context.Context) error {
	return nil
}

func (r *desktopRuntimeOnlyWebRuntime) GetWebWorkerOpenStream(webWorkerID string) srpc.OpenStreamFunc {
	return unavailableRendererOpenStream
}

func unavailableRendererOpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	return nil, errors.New("renderer window is unavailable")
}

func TestComputeElectronExitDisposition(t *testing.T) {
	for _, tc := range []struct {
		name       string
		runtimeErr error
		processErr error
		quitPolicy QuitPolicy
		presence   DesktopPresencePolicy
		want       electronExitDisposition
	}{
		{
			name:       "clean exit window lifetime exit policy stays resident",
			quitPolicy: QuitPolicy_QUIT_POLICY_EXIT,
			presence:   DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
			want:       electronExitStayResident,
		},
		{
			name:       "clean exit window lifetime restart policy stays resident",
			quitPolicy: QuitPolicy_QUIT_POLICY_RESTART,
			presence:   DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
			want:       electronExitStayResident,
		},
		{
			name:       "clean exit tray background exit policy exits host",
			quitPolicy: QuitPolicy_QUIT_POLICY_EXIT,
			presence:   DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
			want:       electronExitHost,
		},
		{
			name:       "clean exit tray background restart policy restarts",
			quitPolicy: QuitPolicy_QUIT_POLICY_RESTART,
			presence:   DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
			want:       electronExitRestart,
		},
		{
			name:       "expected runtime disconnect stays resident under window lifetime",
			runtimeErr: errors.New("stream reset"),
			processErr: context.DeadlineExceeded,
			quitPolicy: QuitPolicy_QUIT_POLICY_EXIT,
			presence:   DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
			want:       electronExitStayResident,
		},
		{
			name:       "non-clean exit restarts under window lifetime",
			runtimeErr: errors.New("unexpected disconnect"),
			processErr: context.DeadlineExceeded,
			quitPolicy: QuitPolicy_QUIT_POLICY_EXIT,
			presence:   DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
			want:       electronExitRestart,
		},
		{
			name:       "non-clean exit restarts under tray background",
			runtimeErr: errors.New("unexpected disconnect"),
			processErr: context.DeadlineExceeded,
			quitPolicy: QuitPolicy_QUIT_POLICY_EXIT,
			presence:   DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
			want:       electronExitRestart,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := computeElectronExitDisposition(tc.runtimeErr, tc.processErr, tc.quitPolicy, tc.presence)
			if got != tc.want {
				t.Fatalf("computeElectronExitDisposition() = %v, want %v", got, tc.want)
			}
		})
	}
}

// _ is a type assertion
var _ web_runtime.WebRuntime = (*desktopRuntimeOnlyWebRuntime)(nil)
